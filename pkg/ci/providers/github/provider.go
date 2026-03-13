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
	client      *Client
	clientOnce  sync.Once
	clientErr   error
	checkRunIDs sync.Map // name → int64 ID, for correlating CreateCheckRun/UpdateCheckRun.
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

	// Set branch name (prefer GITHUB_HEAD_REF for PRs, fall back to GITHUB_REF_NAME).
	branch := os.Getenv("GITHUB_HEAD_REF") // PR head branch.
	if branch == "" {
		branch = os.Getenv("GITHUB_REF_NAME") // Branch name for push events.
	}
	ctx.Branch = branch

	// Parse PR info from GITHUB_REF for pull_request events.
	if ctx.EventName == "pull_request" || ctx.EventName == "pull_request_target" {
		ctx.PullRequest = parsePRInfo()
	}

	return ctx, nil
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

// parsePRInfo extracts PR information from environment variables.
func parsePRInfo() *provider.PRInfo {
	refName := os.Getenv("GITHUB_REF_NAME")
	baseRef := os.Getenv("GITHUB_BASE_REF")
	headRef := os.Getenv("GITHUB_HEAD_REF")

	// Extract PR number from ref (refs/pull/<number>/merge).
	ref := os.Getenv("GITHUB_REF")
	var prNumber int
	if strings.HasPrefix(ref, "refs/pull/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 3 {
			prNumber, _ = strconv.Atoi(parts[2])
		}
	}

	// If GITHUB_REF_NAME is in format "123/merge", extract number.
	if prNumber == 0 && strings.HasSuffix(refName, "/merge") {
		numStr := strings.TrimSuffix(refName, "/merge")
		prNumber, _ = strconv.Atoi(numStr)
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		serverURL = "https://github.com"
	}

	var prURL string
	if prNumber > 0 && repo != "" {
		prURL = serverURL + "/" + repo + "/pull/" + strconv.Itoa(prNumber)
	}

	return &provider.PRInfo{
		Number:  prNumber,
		HeadRef: headRef,
		BaseRef: baseRef,
		URL:     prURL,
	}
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
	// Only register if we can detect GitHub Actions.
	// The client is lazily initialized — GITHUB_TOKEN is not required at init time.
	p := NewProvider()
	if p.Detect() {
		ci.Register(p)
	}
}
