package monitor

import (
	"context"
	"io"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

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

// Check performs a GET request and returns the HTTP status code and how long
// the request took (including retries). On connection error, returns 0, the
// elapsed time, and the error.
func (c *HTTPChecker) Check(ctx context.Context, url string) (int, time.Duration, error) {
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, 0, err
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return 0, latency, err
	}
	defer func() {
		// Drain the body so the connection can be reused (keep-alive).
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	return resp.StatusCode, latency, nil
}
