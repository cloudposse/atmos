//go:generate go run go.uber.org/mock/mockgen@v0.5.0 -source=client.go -destination=mock_client_test.go -package=http

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"

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
func WithGitHubToken(token string) ClientOption {
	defer perf.Track(nil, "http.WithGitHubToken")()

	return func(c *DefaultClient) {
		if token != "" {
			c.client.Transport = &GitHubAuthenticatedTransport{
				Base:        http.DefaultTransport,
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
func GetGitHubTokenFromEnv() string {
	defer perf.Track(nil, "http.GetGitHubTokenFromEnv")()

	if token := viper.GetString("ATMOS_GITHUB_TOKEN"); token != "" {
		return token
	}
	return viper.GetString("GITHUB_TOKEN")
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status code: %d", errUtils.ErrHTTPRequestFailed, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", errors.Join(errUtils.ErrHTTPRequestFailed, err))
	}

	return body, nil
}
