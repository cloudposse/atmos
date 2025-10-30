//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=client.go -destination=mock_client.go -package=http

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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

// ClientOption is a functional option for configuring the DefaultClient.
type ClientOption func(*DefaultClient)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	defer perf.Track(nil, "http.WithTimeout")()

	return func(c *DefaultClient) {
		c.client.Timeout = timeout
	}
}

// WithGitHubToken sets the GitHub token for authenticated requests.
// Wraps the existing transport instead of replacing it to allow composition with WithTransport.
func WithGitHubToken(token string) ClientOption {
	defer perf.Track(nil, "http.WithGitHubToken")()

	return func(c *DefaultClient) {
		if token != "" {
			// Wrap existing transport (or use default if none set).
			base := c.client.Transport
			if base == nil {
				base = http.DefaultTransport
			}
			c.client.Transport = &GitHubAuthenticatedTransport{
				Base:        base,
				GitHubToken: token,
			}
		}
	}
}

// WithTransport sets a custom HTTP transport.
func WithTransport(transport http.RoundTripper) ClientOption {
	defer perf.Track(nil, "http.WithTransport")()

	return func(c *DefaultClient) {
		c.client.Transport = transport
	}
}

// DefaultClient is the default HTTP client implementation.
type DefaultClient struct {
	client *http.Client
}

// NewDefaultClient creates a new DefaultClient with optional configuration.
func NewDefaultClient(opts ...ClientOption) *DefaultClient {
	defer perf.Track(nil, "http.NewDefaultClient")()

	client := &DefaultClient{
		client: &http.Client{
			Timeout: 30 * time.Second, // Default timeout
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// GitHubAuthenticatedTransport wraps an http.Transport to add GitHub token authentication.
type GitHubAuthenticatedTransport struct {
	Base        http.RoundTripper
	GitHubToken string
}

// RoundTrip implements http.RoundTripper interface.
func (t *GitHubAuthenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "http.GitHubAuthenticatedTransport.RoundTrip")()

	host := req.URL.Hostname()
	if (host == "api.github.com" || host == "raw.githubusercontent.com") && t.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+t.GitHubToken)
		req.Header.Set("User-Agent", "atmos-toolchain/1.0")
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub transport roundtrip: %w", err)
	}

	return resp, nil
}

// GetGitHubTokenFromEnv retrieves GitHub token from environment variables.
// Checks ATMOS_GITHUB_TOKEN first, then falls back to GITHUB_TOKEN.
// Uses os.Getenv directly to avoid requiring viper.BindEnv in library code.
func GetGitHubTokenFromEnv() string {
	defer perf.Track(nil, "http.GetGitHubTokenFromEnv")()

	if token := os.Getenv("ATMOS_GITHUB_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("GITHUB_TOKEN")
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

		return nil, fmt.Errorf("%w: %s %s returned status %d, content-type: %s, response body%s:\n%s",
			errUtils.ErrHTTPRequestFailed, req.Method, req.URL.Redacted(), resp.StatusCode,
			resp.Header.Get("Content-Type"), truncated, string(errorBody))
	}

	// Success case: Read full response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s failed to read response: %w", req.Method, req.URL.Redacted(), errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	return body, nil
}
