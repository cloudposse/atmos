package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// GitHub API status values.
const (
	statusQueued     = "queued"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
)

// createCheckRun creates a new check run on a commit.
func (p *Provider) createCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	ghOpts := github.CreateCheckRunOptions{
		Name:    opts.Name,
		HeadSHA: opts.SHA,
	}

	// Map status to GitHub API values.
	switch opts.Status {
	case provider.CheckRunStatePending:
		ghOpts.Status = github.String(statusQueued)
	case provider.CheckRunStateInProgress:
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

	return &provider.CheckRun{
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
func (p *Provider) updateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	ghOpts := github.UpdateCheckRunOptions{
		Name: opts.Name, // Name is required for updates.
	}

	// Map status to GitHub API values.
	switch opts.Status {
	case provider.CheckRunStatePending:
		ghOpts.Status = github.String(statusQueued)
	case provider.CheckRunStateInProgress:
		ghOpts.Status = github.String(statusInProgress)
	case provider.CheckRunStateSuccess, provider.CheckRunStateFailure, provider.CheckRunStateError, provider.CheckRunStateCancelled:
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

	return &provider.CheckRun{
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

// mapGitHubStatusToCheckRunState maps GitHub API status to provider.CheckRunState.
// Note: For completed status, this returns CheckRunStateSuccess as a fallback.
// When a check run is completed, callers should also examine the conclusion
// field to determine the actual outcome (success, failure, cancelled, etc.).
func mapGitHubStatusToCheckRunState(status string) provider.CheckRunState {
	switch status {
	case statusQueued:
		return provider.CheckRunStatePending
	case statusInProgress:
		return provider.CheckRunStateInProgress
	case statusCompleted:
		// Completed status requires conclusion to determine actual state.
		// This is a fallback; callers should check the Conclusion field.
		// GitHub conclusions: success, failure, neutral, cancelled, skipped,
		// timed_out, action_required.
		return provider.CheckRunStateSuccess
	default:
		return provider.CheckRunStatePending
	}
}

// mapCheckRunStateToConclusion maps provider.CheckRunState to GitHub API conclusion.
func mapCheckRunStateToConclusion(state provider.CheckRunState, providedConclusion string) string {
	if providedConclusion != "" {
		return providedConclusion
	}

	switch state {
	case provider.CheckRunStateSuccess:
		return "success"
	case provider.CheckRunStateFailure:
		return "failure"
	case provider.CheckRunStateError:
		return "failure"
	case provider.CheckRunStateCancelled:
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
