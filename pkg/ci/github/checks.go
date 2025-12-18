package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
)

// GitHub API status values.
const (
	statusQueued     = "queued"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

// createCheckRun creates a new check run on a commit.
func (p *Provider) createCheckRun(ctx context.Context, opts *ci.CreateCheckRunOptions) (*ci.CheckRun, error) {
	ghOpts := github.CreateCheckRunOptions{
		Name:    opts.Name,
		HeadSHA: opts.SHA,
	}

	// Map status to GitHub API values.
	switch opts.Status {
	case ci.CheckRunStatePending:
		ghOpts.Status = github.String(statusQueued)
	case ci.CheckRunStateInProgress:
		ghOpts.Status = github.String(statusInProgress)
	default:
		ghOpts.Status = github.String(statusQueued)
	}

	// Add output if title or summary is provided.
	if opts.Title != "" || opts.Summary != "" {
		ghOpts.Output = &github.CheckRunOutput{
			Title:   github.String(opts.Title),
			Summary: github.String(opts.Summary),
		}
	}

	if opts.DetailsURL != "" {
		ghOpts.DetailsURL = github.String(opts.DetailsURL)
	}

	if opts.ExternalID != "" {
		ghOpts.ExternalID = github.String(opts.ExternalID)
	}

	checkRun, _, err := p.client.GitHub().Checks.CreateCheckRun(ctx, opts.Owner, opts.Repo, ghOpts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunCreateFailed, err)
	}

	return &ci.CheckRun{
		ID:         checkRun.GetID(),
		Name:       checkRun.GetName(),
		Status:     mapGitHubStatusToCheckRunState(checkRun.GetStatus()),
		Conclusion: checkRun.GetConclusion(),
		Title:      getCheckRunOutputTitle(checkRun),
		Summary:    getCheckRunOutputSummary(checkRun),
		DetailsURL: checkRun.GetDetailsURL(),
		StartedAt:  checkRun.GetStartedAt().Time,
	}, nil
}

// updateCheckRun updates an existing check run.
func (p *Provider) updateCheckRun(ctx context.Context, opts *ci.UpdateCheckRunOptions) (*ci.CheckRun, error) {
	ghOpts := github.UpdateCheckRunOptions{
		Name: opts.Title, // Name is required for updates.
	}

	// Map status to GitHub API values.
	switch opts.Status {
	case ci.CheckRunStatePending:
		ghOpts.Status = github.String(statusQueued)
	case ci.CheckRunStateInProgress:
		ghOpts.Status = github.String(statusInProgress)
	case ci.CheckRunStateSuccess, ci.CheckRunStateFailure, ci.CheckRunStateError, ci.CheckRunStateCancelled:
		ghOpts.Status = github.String(statusCompleted)
		ghOpts.Conclusion = github.String(mapCheckRunStateToConclusion(opts.Status, opts.Conclusion))
	}

	// Add output if title or summary is provided.
	if opts.Title != "" || opts.Summary != "" {
		ghOpts.Output = &github.CheckRunOutput{
			Title:   github.String(opts.Title),
			Summary: github.String(opts.Summary),
		}
	}

	if opts.CompletedAt != nil {
		ghOpts.CompletedAt = &github.Timestamp{Time: *opts.CompletedAt}
	}

	checkRun, _, err := p.client.GitHub().Checks.UpdateCheckRun(ctx, opts.Owner, opts.Repo, opts.CheckRunID, ghOpts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrCICheckRunUpdateFailed, err)
	}

	var completedAt time.Time
	if checkRun.CompletedAt != nil {
		completedAt = checkRun.CompletedAt.Time
	}

	return &ci.CheckRun{
		ID:          checkRun.GetID(),
		Name:        checkRun.GetName(),
		Status:      mapGitHubStatusToCheckRunState(checkRun.GetStatus()),
		Conclusion:  checkRun.GetConclusion(),
		Title:       getCheckRunOutputTitle(checkRun),
		Summary:     getCheckRunOutputSummary(checkRun),
		DetailsURL:  checkRun.GetDetailsURL(),
		StartedAt:   checkRun.GetStartedAt().Time,
		CompletedAt: completedAt,
	}, nil
}

// mapGitHubStatusToCheckRunState maps GitHub API status to ci.CheckRunState.
func mapGitHubStatusToCheckRunState(status string) ci.CheckRunState {
	switch status {
	case statusQueued:
		return ci.CheckRunStatePending
	case statusInProgress:
		return ci.CheckRunStateInProgress
	case statusCompleted:
		// Note: completed status requires conclusion to determine actual state.
		return ci.CheckRunStateSuccess
	default:
		return ci.CheckRunStatePending
	}
}

// mapCheckRunStateToConclusion maps ci.CheckRunState to GitHub API conclusion.
func mapCheckRunStateToConclusion(state ci.CheckRunState, providedConclusion string) string {
	if providedConclusion != "" {
		return providedConclusion
	}

	switch state {
	case ci.CheckRunStateSuccess:
		return "success"
	case ci.CheckRunStateFailure:
		return "failure"
	case ci.CheckRunStateError:
		return "failure"
	case ci.CheckRunStateCancelled:
		return "cancelled"
	default:
		return "neutral"
	}
}

// getCheckRunOutputTitle safely extracts the title from check run output.
func getCheckRunOutputTitle(cr *github.CheckRun) string {
	if cr.Output != nil {
		return cr.Output.GetTitle()
	}
	return ""
}

// getCheckRunOutputSummary safely extracts the summary from check run output.
func getCheckRunOutputSummary(cr *github.CheckRun) string {
	if cr.Output != nil {
		return cr.Output.GetSummary()
	}
	return ""
}
