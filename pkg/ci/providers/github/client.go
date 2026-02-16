// Package github provides GitHub Actions CI provider implementation.
package github

import (
	"context"
	"net/http"
	"os"

	"github.com/google/go-github/v59/github"
	"golang.org/x/oauth2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Client wraps the GitHub API client.
type Client struct {
	client *github.Client
}

// NewClient creates a new GitHub API client.
// It uses the GITHUB_TOKEN environment variable for authentication.
func NewClient() (*Client, error) {
	defer perf.Track(nil, "github.NewClient")()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		// Also check GH_TOKEN (used by gh CLI).
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		return nil, errUtils.ErrGitHubTokenNotFound
	}

	return NewClientWithToken(token), nil
}

// NewClientWithToken creates a new GitHub API client with the given token.
func NewClientWithToken(token string) *Client {
	defer perf.Track(nil, "github.NewClientWithToken")()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	return &Client{
		client: github.NewClient(tc),
	}
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
