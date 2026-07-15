// Package github implements GitHub's pull-request API as a git publishing provider.
package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v59/github" //nolint:depguard // This package is the GitHub-specific pull-request provider.

	errUtils "github.com/cloudposse/atmos/errors"
	githubci "github.com/cloudposse/atmos/pkg/ci/providers/github"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

const ProviderName = "github"

type client interface {
	GitHub() *gh.Client
}

type Provider struct {
	newClient func() (client, error)
}

func init() {
	atmosgit.RegisterPullRequestPublisher(ProviderName, func() (atmosgit.PullRequestPublisher, error) {
		return New(), nil
	})
}

func New() *Provider {
	return &Provider{newClient: func() (client, error) { return githubci.NewClient() }}
}

// NewWithClientFactory exists for fake HTTP client tests without exposing the
// go-github dependency to caller packages.
func NewWithClientFactory(factory func() (client, error)) *Provider {
	return &Provider{newClient: factory}
}

//nolint:cyclop,revive // Reconciliation keeps its forge API calls together at the provider boundary.
func (p *Provider) Reconcile(ctx context.Context, options *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	if options == nil {
		return nil, fmt.Errorf("%w: pull request options are required", errUtils.ErrComponentUpdaterConfig)
	}
	if options.Owner == "" || options.Repository == "" || options.Base == "" || options.Head == "" {
		return nil, fmt.Errorf("%w: owner, repository, base, and head are required", errUtils.ErrComponentUpdaterConfig)
	}
	c, err := p.newClient()
	if err != nil {
		return nil, fmt.Errorf("%w: configure ATMOS_CI_GITHUB_TOKEN, GITHUB_TOKEN, or GH_TOKEN: %w", errUtils.ErrGitHubAuthorization, err)
	}
	api := c.GitHub()
	head := options.Owner + ":" + options.Head
	prs, response, err := api.PullRequests.List(ctx, options.Owner, options.Repository, &gh.PullRequestListOptions{
		State: "open", Head: head, Base: options.Base, ListOptions: gh.ListOptions{PerPage: 100},
	})
	if err != nil {
		return nil, githubError(err, response)
	}

	var pr *gh.PullRequest
	created := false
	if len(prs) > 0 {
		pr = prs[0]
		pr, response, err = api.PullRequests.Edit(ctx, options.Owner, options.Repository, pr.GetNumber(), &gh.PullRequest{
			Title: gh.String(options.Title), Body: gh.String(options.Body),
		})
		if err != nil {
			return nil, githubError(err, response)
		}
	} else {
		pr, response, err = api.PullRequests.Create(ctx, options.Owner, options.Repository, &gh.NewPullRequest{
			Title: gh.String(options.Title), Body: gh.String(options.Body), Head: gh.String(options.Head), Base: gh.String(options.Base), Draft: gh.Bool(options.Draft),
		})
		if err != nil {
			return nil, githubError(err, response)
		}
		created = true
	}

	if len(options.Labels) > 0 {
		if _, response, err = api.Issues.AddLabelsToIssue(ctx, options.Owner, options.Repository, pr.GetNumber(), options.Labels); err != nil {
			return nil, githubError(err, response)
		}
	}
	if len(options.Assignees) > 0 {
		if _, response, err = api.Issues.AddAssignees(ctx, options.Owner, options.Repository, pr.GetNumber(), options.Assignees); err != nil {
			return nil, githubError(err, response)
		}
	}
	if len(options.Reviewers) > 0 {
		if _, response, err = api.PullRequests.RequestReviewers(ctx, options.Owner, options.Repository, pr.GetNumber(), gh.ReviewersRequest{Reviewers: options.Reviewers}); err != nil {
			return nil, githubError(err, response)
		}
	}

	return &atmosgit.PullRequestResult{Number: pr.GetNumber(), URL: pr.GetHTMLURL(), Created: created}, nil
}

func githubError(err error, response *gh.Response) error {
	if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
		return fmt.Errorf("%w: GitHub returned HTTP %d; grant contents: write, pull-requests: write, and issues: write as needed: %w", errUtils.ErrGitHubAuthorization, response.StatusCode, err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "bad credentials") {
		return fmt.Errorf("%w: %w", errUtils.ErrGitHubAuthorization, err)
	}
	return fmt.Errorf("%w: %w", errUtils.ErrPullRequestReconciliation, err)
}
