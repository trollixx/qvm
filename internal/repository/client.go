package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// Client wraps an HTTP client with retry logic and ETag support.
type Client struct {
	inner          *retryablehttp.Client
	timeoutSeconds int
}

// NewClient creates a new repository HTTP client.
func NewClient(timeoutSeconds int) *Client {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	rc := retryablehttp.NewClient()
	rc.RetryMax = 3
	rc.RetryWaitMin = 1 * time.Second
	rc.RetryWaitMax = 8 * time.Second
	rc.Logger = nil // suppress default logging
	rc.HTTPClient.Timeout = time.Duration(timeoutSeconds) * time.Second

	return &Client{inner: rc, timeoutSeconds: timeoutSeconds}
}

// FetchWithETag fetches a URL, sending If-None-Match if etag is non-empty.
// Returns (body, newETag, error). If the server returns 304, body is nil.
func (c *Client) FetchWithETag(ctx context.Context, url, etag string) ([]byte, string, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("building request for %s: %w", url, err)
	}
	req = req.WithContext(ctx)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("reading body of %s: %w", url, err)
	}

	newETag := resp.Header.Get("ETag")
	return body, newETag, nil
}

// FetchBytes fetches a URL and returns its body.
func (c *Client) FetchBytes(ctx context.Context, url string) ([]byte, error) {
	body, _, err := c.FetchWithETag(ctx, url, "")
	return body, err
}

// Head performs an HTTP HEAD request and returns the status code.
func (c *Client) Head(ctx context.Context, url string) (int, error) {
	status, _, err := c.head(ctx, url)
	return status, err
}

// ContentLength returns the size of url, as a HEAD request reports it.
func (c *Client) ContentLength(ctx context.Context, url string) (int64, error) {
	status, length, err := c.head(ctx, url)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d for HEAD %s", status, url)
	}
	if length < 0 {
		return 0, fmt.Errorf("no size for %s", url)
	}
	return length, nil
}

// head performs an HTTP HEAD request and returns the status code and the
// content length, which is -1 when the server doesn't send one.
func (c *Client) head(ctx context.Context, url string) (int, int64, error) {
	req, err := retryablehttp.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("building HEAD request for %s: %w", url, err)
	}
	req = req.WithContext(ctx)

	resp, err := c.inner.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("HEAD %s: %w", url, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, resp.ContentLength, nil
}
