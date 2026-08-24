package monitor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// maxKeywordBodySize caps how much of the response body is read for keyword matching.
const maxKeywordBodySize = 1 << 20 // 1 MiB

// CheckOptions configures per-endpoint check expectations.
type CheckOptions struct {
	ExpectedStatus int    // exact HTTP status required; 0 = any 2xx
	Keyword        string // substring the response body must contain; "" = skip body check
}

// CheckResult describes the outcome of a single health check.
type CheckResult struct {
	Up         bool
	StatusCode int
	Latency    time.Duration
	Reason     string    // human-readable failure reason; "" when Up
	CertExpiry time.Time // TLS certificate expiry; zero for plain HTTP
	Err        error     // transport-level error, if any
}

// SchemeChecker performs health checks, dispatching on the URL scheme:
// http/https use retryablehttp, tcp/dns/ping use the net package.
type SchemeChecker struct {
	client  *retryablehttp.Client
	timeout time.Duration
}

// NewChecker creates a SchemeChecker with retryablehttp configured per DESIGN.md.
func NewChecker(timeout time.Duration) *SchemeChecker {
	client := retryablehttp.NewClient()
	client.RetryMax = 2
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 2 * time.Second
	client.HTTPClient.Timeout = timeout
	client.Logger = nil
	// Return the last response instead of an error after retries exhausted
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	return &SchemeChecker{client: client, timeout: timeout}
}

// Check dispatches on the URL scheme: tcp://, dns://, and ping:// use
// scheme-specific probes (CheckOptions expectations apply to HTTP only);
// anything else is treated as HTTP(S).
func (c *SchemeChecker) Check(ctx context.Context, url string, opts CheckOptions) CheckResult {
	scheme, _, _ := strings.Cut(url, "://")
	switch scheme {
	case "tcp":
		return c.checkTCP(ctx, url)
	case "dns":
		return c.checkDNS(ctx, url)
	case "ping":
		return c.checkPing(ctx, url)
	default:
		return c.checkHTTP(ctx, url, opts)
	}
}

// checkTCP reports up when a TCP connection to the host:port target succeeds.
func (c *SchemeChecker) checkTCP(ctx context.Context, rawURL string) CheckResult {
	res := CheckResult{}
	addr := strings.TrimPrefix(rawURL, "tcp://")

	start := time.Now()
	conn, err := (&net.Dialer{Timeout: c.timeout}).DialContext(ctx, "tcp", addr)
	res.Latency = time.Since(start)
	if err != nil {
		res.Reason = "connection error"
		res.Err = err
		return res
	}
	_ = conn.Close()

	res.Up = true
	return res
}

// checkDNS reports up when the hostname resolves to at least one address.
func (c *SchemeChecker) checkDNS(ctx context.Context, rawURL string) CheckResult {
	res := CheckResult{}
	host := strings.TrimPrefix(rawURL, "dns://")

	start := time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	res.Latency = time.Since(start)
	if err != nil {
		res.Reason = "dns lookup failed"
		res.Err = err
		return res
	}
	if len(addrs) == 0 {
		res.Reason = "no DNS records"
		return res
	}

	res.Up = true
	return res
}

// checkPing reports up when the host answers an ICMP echo request. Uses
// unprivileged datagram ICMP (udp4), which requires the host's
// net.ipv4.ping_group_range to allow it (default on most Linux).
func (c *SchemeChecker) checkPing(ctx context.Context, rawURL string) CheckResult {
	res := CheckResult{}
	host := strings.TrimPrefix(rawURL, "ping://")

	ip, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		res.Reason = "dns lookup failed"
		res.Err = err
		return res
	}

	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		res.Reason = "ping not permitted"
		res.Err = err
		return res
	}
	defer conn.Close()

	id := os.Getpid() & 0xffff
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: 1, Data: []byte("noroshi")},
	}
	wb, err := msg.Marshal(nil)
	if err != nil {
		res.Reason = "ping error"
		res.Err = err
		return res
	}

	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		res.Reason = "ping error"
		res.Err = err
		return res
	}

	start := time.Now()
	if _, err := conn.WriteTo(wb, &net.UDPAddr{IP: ip.IP}); err != nil {
		res.Latency = time.Since(start)
		res.Reason = "connection error"
		res.Err = err
		return res
	}

	// Read until the matching echo reply arrives (or the deadline passes);
	// unrelated packets (other processes' pings) are skipped.
	rb := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(rb)
		res.Latency = time.Since(start)
		if err != nil {
			res.Reason = "ping timeout"
			res.Err = err
			return res
		}
		reply, err := icmp.ParseMessage(1, rb[:n]) // 1 = ICMPv4 protocol number
		if err != nil {
			continue
		}
		if reply.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if echo, ok := reply.Body.(*icmp.Echo); ok && echo.ID == id {
			res.Up = true
			return res
		}
	}
}

// checkHTTP performs a GET request and evaluates the response against opts.
// Transport errors and unmet expectations both result in Up=false with a
// human-readable Reason.
func (c *SchemeChecker) checkHTTP(ctx context.Context, url string, opts CheckOptions) CheckResult {
	res := CheckResult{}

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		res.Reason = fmt.Sprintf("invalid request: %v", err)
		res.Err = err
		return res
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Reason = "connection error"
		res.Err = err
		return res
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		res.CertExpiry = resp.TLS.PeerCertificates[0].NotAfter
	}

	// Status expectation.
	statusOK := resp.StatusCode >= 200 && resp.StatusCode < 300
	if opts.ExpectedStatus > 0 {
		statusOK = resp.StatusCode == opts.ExpectedStatus
	}
	if !statusOK {
		if opts.ExpectedStatus > 0 {
			res.Reason = fmt.Sprintf("expected HTTP %d, got %d", opts.ExpectedStatus, resp.StatusCode)
		} else {
			res.Reason = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxKeywordBodySize))
		return res
	}

	// Keyword expectation (only checked when the status is acceptable).
	if opts.Keyword != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxKeywordBodySize))
		if err != nil {
			res.Reason = "failed to read response body"
			res.Err = err
			return res
		}
		if ok, reason := matchKeyword(string(body), opts.Keyword); !ok {
			res.Reason = reason
			return res
		}
	} else {
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxKeywordBodySize))
	}

	res.Up = true
	return res
}

// matchKeyword evaluates the body against the keyword spec. Prefixes select
// the mode: "re:" = body must match the regexp, "!re:" = must not match,
// "!" = must not contain the substring, no prefix = must contain it.
// Returns ok=false with a human-readable reason on mismatch or invalid regex.
func matchKeyword(body, keyword string) (bool, string) {
	negated := strings.HasPrefix(keyword, "!")
	spec := strings.TrimPrefix(keyword, "!")

	if pattern, ok := strings.CutPrefix(spec, "re:"); ok {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Sprintf("invalid regex %q", pattern)
		}
		if re.MatchString(body) {
			if negated {
				return false, fmt.Sprintf("forbidden pattern %q matched", pattern)
			}
			return true, ""
		}
		if negated {
			return true, ""
		}
		return false, fmt.Sprintf("pattern %q did not match", pattern)
	}

	if strings.Contains(body, spec) {
		if negated {
			return false, fmt.Sprintf("forbidden keyword %q found", spec)
		}
		return true, ""
	}
	if negated {
		return true, ""
	}
	return false, fmt.Sprintf("keyword %q not found", spec)
}
