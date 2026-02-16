package github

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ProviderName is the name of the GitHub Actions provider.
	ProviderName = "github-actions"
)

// Provider implements provider.Provider for GitHub Actions.
type Provider struct {
	client *Client
}

// NewProvider creates a new GitHub Actions provider.
func NewProvider() (*Provider, error) {
	defer perf.Track(nil, "github.NewProvider")()

	client, err := NewClient()
	if err != nil {
		return nil, err
	}
	return &Provider{client: client}, nil
}

// NewProviderWithClient creates a new GitHub Actions provider with a custom client.
func NewProviderWithClient(client *Client) *Provider {
	defer perf.Track(nil, "github.NewProviderWithClient")()

	return &Provider{client: client}
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
		SHA:        os.Getenv("GITHUB_SHA"),
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

// OutputWriter returns an OutputWriter for GitHub Actions.
func (p *Provider) OutputWriter() provider.OutputWriter {
	defer perf.Track(nil, "github.Provider.OutputWriter")()

	return provider.NewFileOutputWriter(
		os.Getenv("GITHUB_OUTPUT"),
		os.Getenv("GITHUB_STEP_SUMMARY"),
	)
}

func init() {
	// Only register if we can detect GitHub Actions.
	// We create a lightweight provider just for detection.
	p := &Provider{}
	if p.Detect() {
		// Create full provider with client for actual use.
		fullProvider, err := NewProvider()
		if err != nil {
			// Log warning but don't fail - CI detection worked but client creation failed.
			// This allows graceful degradation in environments where GitHub API is unavailable.
			log.Debug("Failed to create GitHub provider", "error", err)
			return
		}
		ci.Register(fullProvider)
	}
}
