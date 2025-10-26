//go:generate go run go.uber.org/mock/mockgen@latest -source=client.go -destination=mock_client.go -package=http

package http

import (
	"context"
	"errors"
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
		return nil, fmt.Errorf("failed to create request for %s: %w", url, errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", req.Method, req.URL.Redacted(), errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read limited response body for error reporting (prevent DOS from huge responses).
		// Read maxErrorBodySize+1 to detect truncation.
		limited := io.LimitReader(resp.Body, maxErrorBodySize+1)
		errorBody, readErr := io.ReadAll(limited)
		if readErr != nil {
			return nil, fmt.Errorf("%s %s returned status %d (failed to read error body: %w)",
				req.Method, req.URL.Redacted(), resp.StatusCode, errors.Join(errUtils.ErrHTTPRequestFailed, readErr))
		}

		// Truncate marker if we exceeded the limit.
		truncated := ""
		if len(errorBody) > maxErrorBodySize {
			truncated = " [truncated]"
			errorBody = errorBody[:maxErrorBodySize]
		}

		return nil, fmt.Errorf("%s %s returned status %d, content-type: %s, response body%s:\n%s: %w",
			req.Method, req.URL.Redacted(), resp.StatusCode,
			resp.Header.Get("Content-Type"), truncated, string(errorBody), errUtils.ErrHTTPRequestFailed)
	}

	// Success case: Read full response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed to read response: %w", req.Method, req.URL.Redacted(), errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	return body, nil
}
