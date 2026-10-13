// Package github provides GitHub Actions CI provider implementation.
package github

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-github/v59/github"
	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Client wraps the GitHub API client.
type Client struct {
	client         *github.Client
	tokenTransport *httpsTokenTransport
}

// NewClient creates a new GitHub API client.
// Token precedence: ATMOS_CI_GITHUB_TOKEN > ATMOS_PRO_GITHUB_TOKEN > GITHUB_TOKEN > GH_TOKEN.
// ATMOS_CI_GITHUB_TOKEN allows using a separate token for CI operations
// (e.g., commit statuses) while GITHUB_TOKEN is used by Terraform.
// ATMOS_PRO_GITHUB_TOKEN is a JIT GitHub App installation token brokered by
// Atmos Pro's github/sts auth integration (see pkg/auth/integrations/github);
// unlike GITHUB_TOKEN, pushes/PRs authenticated with it trigger downstream
// GitHub Actions workflows, since it isn't subject to GitHub's anti-recursion
// restriction on the default Actions token.
//
// API base URL precedence: ATMOS_CI_GITHUB_API_URL (explicit override) >
// GITHUB_API_URL (exported natively by GitHub Actions, and set to the
// enterprise API URL on GitHub Enterprise Server runners) > the public
// https://api.github.com default. An unparsable value returns an error wrapping
// ErrInvalidURL rather than silently sending the token to the wrong host.
func NewClient() (*Client, error) {
	defer perf.Track(nil, "github.NewClient")()

	token := os.Getenv("ATMOS_CI_GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("ATMOS_PRO_GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		// Also check GH_TOKEN (used by gh CLI).
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return nil, errUtils.ErrGitHubTokenNotFound
	}

	client := NewClientWithToken(token)
	if err := client.applyAPIURLFromEnv(); err != nil {
		return nil, err
	}

	return client, nil
}

// applyAPIURLFromEnv points the underlying go-github client at the API base URL
// from ATMOS_CI_GITHUB_API_URL or GITHUB_API_URL. It is a no-op when neither is set.
func (c *Client) applyAPIURLFromEnv() error {
	envName, raw := lookupAPIURLEnv()
	if raw == "" {
		return nil
	}

	baseURL, err := parseAPIBaseURL(raw)
	if err != nil {
		return fmt.Errorf("%w: %s=%q: %w", errUtils.ErrInvalidURL, envName, raw, err)
	}

	c.client.BaseURL = baseURL
	c.client.UploadURL = baseURL
	c.tokenTransport.apiURL = baseURL

	return nil
}

// lookupAPIURLEnv returns the name and value of the first non-empty API URL variable.
func lookupAPIURLEnv() (string, string) {
	// ATMOS_CI_GITHUB_API_URL and GITHUB_API_URL are external CI environment
	// variables (GITHUB_API_URL is exported by the Actions runner), not Atmos
	// configuration keys, so os.Getenv is appropriate here.
	for _, name := range []string{"ATMOS_CI_GITHUB_API_URL", "GITHUB_API_URL"} {
		if value := os.Getenv(name); value != "" {
			return name, value
		}
	}

	return "", ""
}

// parseAPIBaseURL parses raw as an absolute http(s) URL and ensures the trailing
// slash that go-github requires on BaseURL.
func parseAPIBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("%w: must be an absolute http(s) URL", errUtils.ErrInvalidURL)
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}

	return parsed, nil
}

// NewClientWithToken creates a new GitHub API client with the given token.
func NewClientWithToken(token string) *Client {
	defer perf.Track(nil, "github.NewClientWithToken")()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	transport := &httpsTokenTransport{Transport: oauth2.Transport{Source: ts}}
	client := github.NewClient(&http.Client{Transport: transport})
	transport.apiURL = client.BaseURL
	return &Client{client: client, tokenTransport: transport}
}

// httpsTokenTransport applies the token policy to every request, including redirects.
// Only the configured HTTPS API host and port receive authentication headers.
// Local HTTP endpoints remain usable, but must never receive authentication headers.
type httpsTokenTransport struct {
	oauth2.Transport
	apiURL *url.URL
}

func (t *httpsTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "github.httpsTokenTransport.RoundTrip")()

	if req.URL.Scheme == "https" && t.apiURL.Scheme == "https" && strings.EqualFold(req.URL.Host, t.apiURL.Host) {
		return t.Transport.RoundTrip(req)
	}
	// Redirects may copy credentials from a previous request; strip them on a clone.
	unauthenticated := req.Clone(req.Context())
	unauthenticated.Header.Del("Authorization")
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(unauthenticated)
}

// NewClientWithHTTPClient creates a new GitHub API client with a custom HTTP client.
// Useful for testing.
func NewClientWithHTTPClient(httpClient *http.Client) *Client {
	defer perf.Track(nil, "github.NewClientWithHTTPClient")()

	return &Client{
		client: github.NewClient(httpClient),
	}
}

// GitHub returns the underlying go-github client.
func (c *Client) GitHub() *github.Client {
	defer perf.Track(nil, "github.Client.GitHub")()

	return c.client
}
