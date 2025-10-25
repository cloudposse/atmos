//go:generate go run go.uber.org/mock/mockgen@v0.5.0 -source=client.go -destination=mock_client_test.go -package=http

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

// Get performs an HTTP GET request with context using the provided client.
func Get(ctx context.Context, url string, client Client) ([]byte, error) {
	defer perf.Track(nil, "http.Get")()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}
	defer resp.Body.Close()

	// Read response body (needed for both success and error cases).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	if resp.StatusCode != http.StatusOK {
		// Include full response body in error message for debugging.
		// Let the terminal handle formatting - don't truncate or sanitize.
		return nil, fmt.Errorf("%w: unexpected status code: %d, content-type: %s, response body:\n%s", errUtils.ErrHTTPRequestFailed, resp.StatusCode, resp.Header.Get("Content-Type"), string(body))
	}

	return body, nil
}
