package github

import (
	"context"

	"github.com/google/go-github/v59/github"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

// Client interface for GitHub operations to enable mocking.
type Client interface {
	ListIssueComments(ctx context.Context, owner, repo string, issueNumber int, opts *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error)
	CreateComment(ctx context.Context, owner, repo string, issueNumber int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
	UpdateComment(ctx context.Context, owner, repo string, commentID int64, comment *github.IssueComment) (*github.IssueComment, *github.Response, error)
}

// RealClient wraps the actual GitHub client.
type RealClient struct {
	client *github.Client
}

// NewClient creates a new GitHub client with authentication.
func NewClient(token string) Client {
	if token == "" {
		_ = viper.BindEnv("GITHUB_TOKEN")
		token = viper.GetString("GITHUB_TOKEN")
	}

	if token == "" {
		// Return unauthenticated client
		return &RealClient{
			client: github.NewClient(nil),
		}
	}

	// Create authenticated client
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)

	return &RealClient{
		client: github.NewClient(tc),
	}
}

// ListIssueComments lists comments for an issue with pagination support.
func (c *RealClient) ListIssueComments(ctx context.Context, owner, repo string, issueNumber int, opts *github.IssueListCommentsOptions) ([]*github.IssueComment, *github.Response, error) {
	return c.client.Issues.ListComments(ctx, owner, repo, issueNumber, opts)
}

// CreateComment creates a new comment on an issue.
func (c *RealClient) CreateComment(ctx context.Context, owner, repo string, issueNumber int, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	return c.client.Issues.CreateComment(ctx, owner, repo, issueNumber, comment)
}

// UpdateComment updates an existing comment.
func (c *RealClient) UpdateComment(ctx context.Context, owner, repo string, commentID int64, comment *github.IssueComment) (*github.IssueComment, *github.Response, error) {
	return c.client.Issues.EditComment(ctx, owner, repo, commentID, comment)
}
