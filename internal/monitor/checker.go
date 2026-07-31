package monitor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
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

// HTTPChecker performs HTTP health checks using retryablehttp.
type HTTPChecker struct {
	client *retryablehttp.Client
}

// NewHTTPChecker creates a HTTPChecker with retryablehttp configured per DESIGN.md.
func NewHTTPChecker(timeout time.Duration) *HTTPChecker {
	client := retryablehttp.NewClient()
	client.RetryMax = 2
	client.RetryWaitMin = 500 * time.Millisecond
	client.RetryWaitMax = 2 * time.Second
	client.HTTPClient.Timeout = timeout
	client.Logger = nil
	// Return the last response instead of an error after retries exhausted
	client.ErrorHandler = retryablehttp.PassthroughErrorHandler
	return &HTTPChecker{client: client}
}

// Check performs a GET request and evaluates the response against opts.
// Transport errors and unmet expectations both result in Up=false with a
// human-readable Reason.
func (c *HTTPChecker) Check(ctx context.Context, url string, opts CheckOptions) CheckResult {
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
		if !strings.Contains(string(body), opts.Keyword) {
			res.Reason = fmt.Sprintf("keyword %q not found", opts.Keyword)
			return res
		}
	} else {
		// Drain so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxKeywordBodySize))
	}

	res.Up = true
	return res
}
