package toolchain

import (
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
	// Add GitHub token to requests to GitHub API and raw content
	if t.isGitHubRequest(req.URL.String()) {
		req.Header.Set("Authorization", "Bearer "+t.GitHubToken)
		// Also set User-Agent to identify the client
		req.Header.Set("User-Agent", "atmos-toolchain/1.0")
	}

	return t.Base.RoundTrip(req)
}

// isGitHubRequest checks if the request is to a GitHub domain that requires authentication.
func (t *GitHubAuthenticatedTransport) isGitHubRequest(url string) bool {
	// Only apply authentication to GitHub API requests, not raw content
	return contains(url, "api.github.com") ||
		(contains(url, "github.com") && !contains(url, "raw.githubusercontent.com"))
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

// containsSubstring checks if a string contains a substring (simplified).
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
