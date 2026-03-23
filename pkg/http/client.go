//go:generate go run go.uber.org/mock/mockgen@latest -source=client.go -destination=mock_client.go -package=http

package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// UserAgent is the User-Agent header value for HTTP requests.
	userAgent = "atmos-toolchain/1.0"

	// MaxErrorBodySize limits how much of an HTTP error response body to include in error messages.
	// This prevents log pollution and potential exposure of large sensitive payloads.
	maxErrorBodySize = 64 * 1024 // 64 KB

	// DefaultTimeoutSeconds is the default HTTP client timeout.
	defaultTimeoutSeconds = 30
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
//
// Triple-composition caveat: if a second WithTransport call follows this option, the
// earlier base transport is silently replaced. See WithTransport for details.
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

// WithGitHubHostMatcher sets a custom host-matching predicate on the GitHub authenticated
// transport. The predicate receives the request hostname (without port) and returns true
// when the host should receive GitHub authentication headers.
//
// This is useful for GitHub Enterprise Server (GHES) deployments or custom GitHub proxies
// where the API is hosted on a non-standard domain.
//
// Example usage:
//
//	client := NewDefaultClient(
//	    WithGitHubToken("token"),
//	    WithGitHubHostMatcher(func(host string) bool {
//	        return host == "github.mycorp.example.com"
//	    }),
//	)
//
// If this option is applied before WithGitHubToken, it has no effect because there is no
// transport to configure yet. Apply it after WithGitHubToken.
func WithGitHubHostMatcher(matcher func(string) bool) ClientOption {
	defer perf.Track(nil, "http.WithGitHubHostMatcher")()

	return func(c *DefaultClient) {
		if authTransport, ok := c.client.Transport.(*GitHubAuthenticatedTransport); ok {
			authTransport.hostMatcher = matcher
		}
	}
}

// WithTransport sets a custom HTTP transport.
// If a GitHubAuthenticatedTransport has already been applied (e.g., via WithGitHubToken),
// the provided transport is set as its Base rather than replacing the auth wrapper.
// This preserves GitHub authentication regardless of option order.
//
// Triple-composition note: when a second WithTransport call follows WithGitHubToken, the
// earlier base transport (from the first WithTransport) is silently replaced by the new one.
// Example: WithTransport(t1), WithGitHubToken("x"), WithTransport(t2)
// Result:  GitHubAuthenticatedTransport{Base: t2, Token: "x"}; t1 is discarded.
func WithTransport(transport http.RoundTripper) ClientOption {
	defer perf.Track(nil, "http.WithTransport")()

	return func(c *DefaultClient) {
		if authTransport, ok := c.client.Transport.(*GitHubAuthenticatedTransport); ok {
			authTransport.Base = transport
		} else {
			c.client.Transport = transport
		}
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
			Timeout: defaultTimeoutSeconds * time.Second,
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

	// hostMatcher is an optional custom predicate that decides whether a given hostname
	// should receive GitHub authentication headers. If nil, the default allowlist is used.
	// See WithGitHubHostMatcher for details.
	hostMatcher func(string) bool
}

// isGitHubHost is the default host allowlist.
// It is also used as the fallback when GitHubAuthenticatedTransport.hostMatcher is nil.
func isGitHubHost(host string) bool {
	// Respect GITHUB_API_URL for GitHub Enterprise Server (GHES) and similar deployments.
	// When set, the hostname of GITHUB_API_URL is treated as an allowed GitHub API host.
	//nolint:forbidigo // Direct env lookup required for GHES configuration.
	if apiURL := os.Getenv("GITHUB_API_URL"); apiURL != "" {
		parsed, err := url.ParseRequestURI(apiURL)
		if err == nil && parsed.Hostname() == host {
			return true
		}
	}

	return host == "api.github.com" || host == "raw.githubusercontent.com"
}

// RoundTrip implements http.RoundTripper interface.
func (t *GitHubAuthenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "http.GitHubAuthenticatedTransport.RoundTrip")()

	// Clone request to avoid mutating caller's request.
	reqClone := req.Clone(req.Context())

	host := reqClone.URL.Hostname()
	scheme := reqClone.URL.Scheme

	// Determine whether the host is allowed to receive authentication headers.
	matcher := t.hostMatcher
	if matcher == nil {
		matcher = isGitHubHost
	}

	// Only inject Authorization when ALL of the following are true:
	//   1. The scheme is "https" (prevent token leakage over plain HTTP).
	//   2. The host is in the allowed list.
	//   3. The header is not already set (outermost transport wins on multi-layer composition).
	if scheme == "https" && matcher(host) && t.GitHubToken != "" {
		if reqClone.Header.Get("Authorization") == "" {
			reqClone.Header.Set("Authorization", "Bearer "+t.GitHubToken)
		}
		reqClone.Header.Set("User-Agent", userAgent)
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(reqClone)
	if err != nil {
		return nil, fmt.Errorf("GitHub transport roundtrip: %w", err)
	}

	return resp, nil
}

// GetGitHubTokenFromEnv retrieves GitHub token from the global configuration.
// This function respects the standard Atmos precedence order:
//  1. --github-token CLI flag (via viper, only available for toolchain commands)
//  2. ATMOS_GITHUB_TOKEN environment variable
//  3. GITHUB_TOKEN environment variable
//
// The viper binding is configured in cmd/toolchain/toolchain.go for toolchain commands.
// For non-toolchain commands, we fall back to direct environment variable lookup.
//
// An optional *viper.Viper instance may be passed; when provided it is used instead of
// the global viper singleton. This is primarily useful in tests to avoid mutating
// shared global state.
func GetGitHubTokenFromEnv(v ...*viper.Viper) string {
	defer perf.Track(nil, "http.GetGitHubTokenFromEnv")()

	viperInst := viper.GetViper()
	if len(v) > 0 && v[0] != nil {
		viperInst = v[0]
	}

	// First try viper (for toolchain commands with --github-token flag).
	if token := viperInst.GetString("github-token"); token != "" {
		return token
	}

	// Fall back to direct environment variable lookup for non-toolchain commands.
	// Check ATMOS_GITHUB_TOKEN first (Atmos-specific), then GITHUB_TOKEN (standard).
	//nolint:forbidigo // Direct env lookup required for non-toolchain commands.
	if token := os.Getenv("ATMOS_GITHUB_TOKEN"); token != "" {
		return token
	}

	//nolint:forbidigo // Direct env lookup required for non-toolchain commands.
	return os.Getenv("GITHUB_TOKEN")
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
