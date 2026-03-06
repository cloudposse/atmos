package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
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

	result := &provider.CheckRun{
		ID:         checkRun.GetID(),
		Name:       checkRun.GetName(),
		Status:     mapGitHubStatusToCheckRunState(checkRun.GetStatus()),
		Conclusion: checkRun.GetConclusion(),
		Title:      getCheckRunOutputTitle(checkRun),
		Summary:    getCheckRunOutputSummary(checkRun),
		DetailsURL: checkRun.GetDetailsURL(),
		StartedAt:  checkRun.GetStartedAt().Time,
	}

	// Store name → ID for later UpdateCheckRun correlation.
	p.checkRunIDs.Store(opts.Name, result.ID)

	return result, nil
}

// updateCheckRun updates an existing check run.
// It resolves the check run ID from the internal name→ID map.
// If no prior CreateCheckRun was called for this name, it falls back to
// creating a new completed check run using opts.SHA.
func (p *Provider) updateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	// Look up the check run ID from the internal map (stored by CreateCheckRun).
	val, ok := p.checkRunIDs.LoadAndDelete(opts.Name)
	if !ok {
		// No prior CreateCheckRun — fall back to creating a completed check run.
		return p.createCompletedCheckRun(ctx, opts)
	}

	checkRunID, _ := val.(int64)

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

	checkRun, _, err := p.client.GitHub().Checks.UpdateCheckRun(ctx, opts.Owner, opts.Repo, checkRunID, ghOpts)
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

// createCompletedCheckRun creates a new completed check run as a fallback
// when UpdateCheckRun is called without a prior CreateCheckRun.
func (p *Provider) createCompletedCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	createOpts := &provider.CreateCheckRunOptions{
		Owner:   opts.Owner,
		Repo:    opts.Repo,
		SHA:     opts.SHA,
		Name:    opts.Name,
		Status:  opts.Status,
		Title:   opts.Title,
		Summary: opts.Summary,
	}

	return p.createCheckRun(ctx, createOpts)
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

// CreateCheckRun creates a new check run on a commit.
func (p *Provider) CreateCheckRun(ctx context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "github.Provider.CreateCheckRun")()

	return p.createCheckRun(ctx, opts)
}

// UpdateCheckRun updates an existing check run.
func (p *Provider) UpdateCheckRun(ctx context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "github.Provider.UpdateCheckRun")()

	return p.updateCheckRun(ctx, opts)
}
