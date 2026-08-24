package github

import (
	"context"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxDescriptionLength is the GitHub Status API limit for the description field.
const maxDescriptionLength = 140

// setCommitStatus sets a commit status on a commit using the GitHub Status API.
// Both CreateCheckRun and UpdateCheckRun delegate to this method since the
// Status API is idempotent by context string — there is no distinction between
// create and update.
func (p *Provider) setCommitStatus(ctx context.Context, owner, repo, sha, statusContext, state, description, targetURL string) (*github.RepoStatus, error) {
	if err := p.ensureClient(); err != nil {
		return nil, err
	}

	repoStatus := &github.RepoStatus{
		State:       github.String(state),
		Context:     github.String(statusContext),
		Description: github.String(truncateDescription(description)),
	}

	if targetURL != "" {
		repoStatus.TargetURL = github.String(targetURL)
	}

	status, _, err := p.client.GitHub().Repositories.CreateStatus(ctx, owner, repo, sha, repoStatus)
	if err != nil {
		return nil, wrapGitHubAPIError(err)
	}
	return status, nil
}

// wrapGitHubAPIError wraps GitHub API errors with actionable hints for common
// permission-related failures (404, 403).
func wrapGitHubAPIError(err error) error {
	var ghErr *github.ErrorResponse
	if !errors.As(err, &ghErr) || ghErr.Response == nil {
		return err
	}

	switch ghErr.Response.StatusCode {
	case http.StatusNotFound:
		return errUtils.Build(err).
			WithHint("A 404 from the GitHub Status API usually means the token lacks permission to create commit statuses.").
			WithHint("If GITHUB_TOKEN is a GitHub App token, the App must have 'Commit statuses: Read and write' permission. The workflow-level 'permissions: statuses: write' only applies to the default GITHUB_TOKEN.").
			WithHint("Set ATMOS_CI_GITHUB_TOKEN to use a separate token for CI operations (e.g., the workflow's default token) while keeping GITHUB_TOKEN for Terraform.").
			Err()
	case http.StatusForbidden:
		return errUtils.Build(err).
			WithHint("The token does not have permission to create commit statuses on this repository.").
			WithHint("Set ATMOS_CI_GITHUB_TOKEN to use a separate token with 'statuses: write' permission for CI operations.").
			Err()
	default:
		return err
	}
}

// createCheckRun sets a commit status on a commit.
func (p *Provider) createCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	state := mapCheckRunStateToStatusState(opts.Status)

	status, err := p.setCommitStatus(ctx, opts.Owner, opts.Repo, opts.SHA, opts.Name, state, opts.Title, opts.DetailsURL)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICheckRunCreateFailed).WithCause(err).Err()
	}

	return &provider.CheckRun{
		ID:     status.GetID(),
		Name:   status.GetContext(),
		Status: opts.Status,
		Title:  status.GetDescription(),
	}, nil
}

// updateCheckRun updates a commit status on a commit.
// Since the Status API is idempotent by context, this is just another CreateStatus call.
func (p *Provider) updateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	state := mapCheckRunStateToStatusState(opts.Status)

	status, err := p.setCommitStatus(ctx, opts.Owner, opts.Repo, opts.SHA, opts.Name, state, opts.Title, opts.DetailsURL)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCICheckRunUpdateFailed).WithCause(err).Err()
	}

	return &provider.CheckRun{
		ID:     status.GetID(),
		Name:   status.GetContext(),
		Status: opts.Status,
		Title:  status.GetDescription(),
	}, nil
}

// mapCheckRunStateToStatusState maps CheckRunState to GitHub Status API state.
func mapCheckRunStateToStatusState(state provider.CheckRunState) string {
	switch state {
	case provider.CheckRunStatePending, provider.CheckRunStateInProgress:
		return "pending"
	case provider.CheckRunStateSuccess:
		return "success"
	case provider.CheckRunStateFailure:
		return "failure"
	case provider.CheckRunStateError, provider.CheckRunStateCancelled:
		return "error"
	default:
		return "pending"
	}
}

// truncateDescription truncates a description to 140 characters (GitHub API limit).
// Uses character count (runes), not byte count, to avoid mid-character truncation.
func truncateDescription(desc string) string {
	if utf8.RuneCountInString(desc) <= maxDescriptionLength {
		return desc
	}
	runes := []rune(desc)
	return string(runes[:maxDescriptionLength-3]) + "..."
}

// CreateCheckRun creates a new commit status on a commit.
func (p *Provider) CreateCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "github.Provider.CreateCheckRun")()

	return p.createCheckRun(ctx, opts)
}

// UpdateCheckRun updates an existing commit status on a commit.
func (p *Provider) UpdateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "github.Provider.UpdateCheckRun")()

	return p.updateCheckRun(ctx, opts)
}
