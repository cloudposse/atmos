//go:generate go run go.uber.org/mock/mockgen@latest -source=client.go -destination=mock_client.go -package=http

package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Client defines the interface for making HTTP requests.
// This interface allows for easy mocking in tests.
type Client interface {
	// Do performs an HTTP request and returns the response.
	Do(req *http.Request) (*http.Response, error)
}

// DefaultClient is the default HTTP client implementation.
type DefaultClient struct {
	client *http.Client
}

// NewDefaultClient creates a new DefaultClient with the specified timeout.
func NewDefaultClient(timeout time.Duration) *DefaultClient {
	defer perf.Track(nil, "http.NewDefaultClient")()

	return &DefaultClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Do implements Client.Do.
func (c *DefaultClient) Do(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "http.DefaultClient.Do")()

	return c.client.Do(req)
}

const (
	// MaxErrorBodySize limits how much of an HTTP error response body to include in error messages.
	// This prevents log pollution and potential exposure of large sensitive payloads.
	maxErrorBodySize = 64 * 1024 // 64 KB
)

// Get performs an HTTP GET request with context using the provided client.
func Get(ctx context.Context, url string, client Client) ([]byte, error) {
	defer perf.Track(nil, "http.Get")()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request for %s: %w", errUtils.ErrHTTPRequestFailed, url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s failed: %w", errUtils.ErrHTTPRequestFailed, req.Method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read limited response body for error reporting (prevent DOS from huge responses).
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodySize))
		if readErr != nil {
			return nil, fmt.Errorf("%w: %s %s returned status %d (failed to read error body: %w)",
				errUtils.ErrHTTPRequestFailed, req.Method, url, resp.StatusCode, readErr)
		}

		// Truncate marker if we hit the limit.
		truncated := ""
		if len(errorBody) == maxErrorBodySize {
			truncated = " [truncated]"
		}

		return nil, fmt.Errorf("%w: %s %s returned status %d, content-type: %s, response body%s:\n%s",
			errUtils.ErrHTTPRequestFailed, req.Method, url, resp.StatusCode,
			resp.Header.Get("Content-Type"), truncated, string(errorBody))
	}

	// Success case: Read full response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s failed to read response: %w", errUtils.ErrHTTPRequestFailed, req.Method, url, err)
	}

	return body, nil
}
