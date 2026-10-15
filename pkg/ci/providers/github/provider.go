package github

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/git"
	ghtoken "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ProviderName is the name of the GitHub Actions provider.
	ProviderName = "github-actions"
)

// Provider implements provider.Provider for GitHub Actions.
// The client is lazily initialized on first use, so the provider can be
// registered at init time based on environment detection alone, without
// requiring GITHUB_TOKEN to be available at startup.
type Provider struct {
	client     *Client
	clientOnce sync.Once
	clientErr  error
}

// NewProvider creates a new GitHub Actions provider.
// The GitHub API client is lazily initialized on first use.
func NewProvider() *Provider {
	defer perf.Track(nil, "github.NewProvider")()

	return &Provider{}
}

// NewProviderWithClient creates a new GitHub Actions provider with a custom client.
func NewProviderWithClient(client *Client) *Provider {
	defer perf.Track(nil, "github.NewProviderWithClient")()

	p := &Provider{client: client}
	// Mark client as already initialized so ensureClient() is a no-op.
	p.clientOnce.Do(func() {})
	return p
}

// ensureClient lazily initializes the GitHub API client.
func (p *Provider) ensureClient() error {
	p.clientOnce.Do(func() {
		if p.client != nil {
			return
		}
		client, err := NewClient()
		if err != nil {
			p.clientErr = err
			return
		}
		p.client = client
	})
	return p.clientErr
}

// ValidateConfig fails the run early when the configured API base URL is unusable.
func (p *Provider) ValidateConfig() error {
	defer perf.Track(nil, "github.Provider.ValidateConfig")()

	return ValidateAPIURL()
}

// Name returns the provider name.
func (p *Provider) Name() string {
	defer perf.Track(nil, "github.Provider.Name")()

	return ProviderName
}

// Detect returns true if running in GitHub Actions.
func (p *Provider) Detect() bool {
	defer perf.Track(nil, "github.Provider.Detect")()

	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// Context returns CI metadata from GitHub Actions environment variables.
//
// SHA is the commit that is actually checked out: git HEAD of the working directory, falling back to
// GITHUB_SHA when HEAD cannot be read. Commit statuses and commit comments land on it. By default that
// equals GITHUB_SHA (the merge commit for pull_request events); a workflow that checks out the pull
// request head (for example under pull_request_target) gets the head commit, which is the commit
// a reviewer sees. Tests that need GITHUB_SHA to win run outside a git checkout.
//
// The pull request number comes from the event payload (pull_request.number, or
// workflow_run.pull_requests[0].number), because GitHub sets GITHUB_REF to the base branch for
// pull_request_target and workflow_run. GITHUB_REF is the fallback. A pull request is a fork when its head
// repository's full name differs from the base repository's; a deleted fork or an unreadable payload
// counts as a fork.
func (p *Provider) Context() (*provider.Context, error) {
	defer perf.Track(nil, "github.Provider.Context")()

	runNumber, _ := strconv.Atoi(os.Getenv("GITHUB_RUN_NUMBER"))

	ctx := &provider.Context{
		Provider:   ProviderName,
		RunID:      os.Getenv("GITHUB_RUN_ID"),
		RunNumber:  runNumber,
		Workflow:   os.Getenv("GITHUB_WORKFLOW"),
		Job:        os.Getenv("GITHUB_JOB"),
		Actor:      os.Getenv("GITHUB_ACTOR"),
		EventName:  os.Getenv("GITHUB_EVENT_NAME"),
		Ref:        os.Getenv("GITHUB_REF"),
		SHA:        resolveGitSHA(),
		Repository: os.Getenv("GITHUB_REPOSITORY"),
	}

	// Parse owner and repo from GITHUB_REPOSITORY.
	if repo := ctx.Repository; repo != "" {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			ctx.RepoOwner = parts[0]
			ctx.RepoName = parts[1]
		}
	}

	// Checkout metadata: honor GITHUB_SERVER_URL so GitHub Enterprise
	// clone URLs resolve to the right host.
	ctx.ServerURL = ghtoken.RepoEndpoints().ServerURL
	if ctx.Repository != "" {
		ctx.CloneURL = ctx.ServerURL + "/" + ctx.Repository + ".git"
	}

	ctx.RunURL = runURL(ctx.Repository, ctx.RunID)

	// Set branch name (prefer GITHUB_HEAD_REF for PRs, fall back to GITHUB_REF_NAME).
	branch := os.Getenv("GITHUB_HEAD_REF") // PR head branch.
	if branch == "" {
		branch = os.Getenv("GITHUB_REF_NAME") // Branch name for push events.
	}
	ctx.Branch = branch

	// Parse PR info from the event payload (falling back to GITHUB_REF) for pull_request events.
	if ctx.EventName == "pull_request" || ctx.EventName == "pull_request_target" {
		ctx.PullRequest = pullRequestFromEvent()
	}

	// A workflow_run triggered by a fork carries no pull_request object, so the fork-execution
	// gate would never see it. Surface it as a fork pull request instead of failing open. A pull
	// request that is already known is never replaced by a run that only describes the same one.
	if ctx.EventName == "workflow_run" {
		ctx.PullRequest = mergeWorkflowRunPullRequest(ctx.PullRequest, workflowRunPullRequest(ctx.Repository))
	}

	// pull_request_target and workflow_run run with the base repository's
	// secrets even though the requested checkout may target untrusted fork
	// content. Mark them elevated so the fork-checkout safety gate engages.
	ctx.ElevatedEvent = ctx.EventName == "pull_request_target" || ctx.EventName == "workflow_run"

	return ctx, nil
}

// runURL composes the GitHub Actions run URL from the server URL, repository,
// and run ID. The presence check on the raw GITHUB_SERVER_URL env var (rather
// than RepoEndpoints, which always resolves to a default) is what detects "not
// running in GitHub Actions"; the actual host value comes from RepoEndpoints so
// a GitHub Enterprise Server host is honored. It returns "" when any part is missing.
func runURL(repository, runID string) string {
	if os.Getenv("GITHUB_SERVER_URL") == "" || repository == "" || runID == "" {
		return ""
	}
	return ghtoken.RepoEndpoints().ServerURL + "/" + repository + "/actions/runs/" + runID
}

// resolveGitSHA returns the current commit SHA by first trying git HEAD,
// then falling back to the GITHUB_SHA environment variable.
func resolveGitSHA() string {
	gitRepo := git.NewDefaultGitRepo()
	sha, err := gitRepo.GetCurrentCommitSHA()
	if err == nil && sha != "" {
		return sha
	}
	log.Debug("Failed to resolve SHA from git HEAD, falling back to GITHUB_SHA", "error", err)
	return os.Getenv("GITHUB_SHA")
}

// GetStatus returns the CI status for the current branch.
func (p *Provider) GetStatus(ctx context.Context, opts provider.StatusOptions) (*provider.Status, error) {
	defer perf.Track(nil, "github.Provider.GetStatus")()

	return p.getStatus(ctx, opts)
}

// OutputWriter returns an OutputWriter for GitHub Actions.
func (p *Provider) OutputWriter() provider.OutputWriter {
	defer perf.Track(nil, "github.Provider.OutputWriter")()

	log.Debug("OutputWriter", "GITHUB_OUTPUT", os.Getenv("GITHUB_OUTPUT"), "GITHUB_STEP_SUMMARY", os.Getenv("GITHUB_STEP_SUMMARY"))

	return provider.NewFileOutputWriter(
		os.Getenv("GITHUB_OUTPUT"),
		os.Getenv("GITHUB_STEP_SUMMARY"),
	)
}

func init() {
	// Register unconditionally: registration advertises the provider's
	// capabilities, while Detect() decides whether it is active for the current
	// run (GITHUB_ACTIONS=true). Registering outside a runner lets cache
	// administration (`atmos ci cache list`/`delete`) resolve the GitHub cache
	// backend locally via ci.ResolveAdminCache, without making the provider the
	// detected one. The client is lazily initialized — GITHUB_TOKEN is not
	// required at init time.
	ci.Register(NewProvider())
}
