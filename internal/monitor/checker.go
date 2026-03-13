package monitor

import (
	"context"
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

// Check performs a GET request and returns the HTTP status code.
// On connection error, returns 0 and the error.
func (c *HTTPChecker) Check(ctx context.Context, url string) (int, error) {
	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}
