package toolchain

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/viper"
)

// HTTPClientConfig holds configuration for HTTP clients.
type HTTPClientConfig struct {
	Timeout time.Duration
	// GitHub token for authenticated requests
	GitHubToken string
}

// NewHTTPClient creates a new HTTP client with optional GitHub token authentication.
func NewHTTPClient(config HTTPClientConfig) *http.Client {
	client := &http.Client{
		Timeout: config.Timeout,
	}

	// If GitHub token is provided, wrap the transport to add authentication
	if config.GitHubToken != "" {
		client.Transport = &GitHubAuthenticatedTransport{
			Base:        http.DefaultTransport,
			GitHubToken: config.GitHubToken,
		}
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

// GetGitHubToken retrieves GitHub token from Viper configuration.
func GetGitHubToken() string {
	return viper.GetString("github-token")
}

// NewDefaultHTTPClient creates a new HTTP client with default configuration and GitHub token support.
func NewDefaultHTTPClient() *http.Client {
	return NewHTTPClient(HTTPClientConfig{
		Timeout:     30 * time.Second,
		GitHubToken: GetGitHubToken(),
	})
}
