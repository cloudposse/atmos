package github

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/google/go-github/v59/github"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Artifact-related errors.
var (
	// ErrPRNotFound indicates the PR does not exist.
	ErrPRNotFound = errors.New("pull request not found")

	// ErrNoWorkflowRunFound indicates no successful workflow run was found for the PR.
	ErrNoWorkflowRunFound = errors.New("no successful workflow run found")

	// ErrNoArtifactFound indicates the requested artifact was not found.
	ErrNoArtifactFound = errors.New("artifact not found")

	// ErrNoArtifactForPlatform indicates no artifact exists for the current platform.
	ErrNoArtifactForPlatform = errors.New("no artifact available for current platform")

	// ErrUnsupportedPlatform indicates the current OS is not supported.
	ErrUnsupportedPlatform = errors.New("unsupported platform")
)

const (
	// WorkflowName is the name of the CI workflow that produces build artifacts.
	workflowName = "Tests"

	// GitHub API pagination size.
	perPage = 100

	// HTTP status code for not found.
	statusNotFound = 404
)

// PRArtifactInfo contains information about a PR's build artifact.
type PRArtifactInfo struct {
	// PR number.
	PRNumber int
	// Head SHA of the PR.
	HeadSHA string
	// Workflow run ID that produced the artifact.
	RunID int64
	// Artifact ID.
	ArtifactID int64
	// Artifact name (e.g., "build-artifacts-macos").
	ArtifactName string
	// Size in bytes.
	SizeInBytes int64
	// Download URL (requires authentication).
	DownloadURL string
	// RunStartedAt is when the workflow run started.
	RunStartedAt time.Time
}

// SHAArtifactInfo contains information about a SHA's build artifact.
type SHAArtifactInfo struct {
	// Head SHA of the commit.
	HeadSHA string
	// Workflow run ID that produced the artifact.
	RunID int64
	// Artifact ID.
	ArtifactID int64
	// Artifact name (e.g., "build-artifacts-macos").
	ArtifactName string
	// Size in bytes.
	SizeInBytes int64
	// Download URL (requires authentication).
	DownloadURL string
	// RunStartedAt is when the workflow run started.
	RunStartedAt time.Time
}

// workflowRunInfo contains metadata about a successful workflow run.
type workflowRunInfo struct {
	ID           int64
	RunStartedAt time.Time
}

// PullRequestService defines the interface for pull request operations.
// This allows for mocking in tests.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=artifacts.go -destination=mock_artifacts_test.go -package=github
type PullRequestService interface {
	Get(ctx context.Context, owner string, repo string, number int) (*github.PullRequest, *github.Response, error)
}

// ActionsService defines the interface for GitHub Actions operations.
// This allows for mocking in tests.
type ActionsService interface {
	ListRepositoryWorkflowRuns(ctx context.Context, owner, repo string, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error)
	ListWorkflowRunArtifacts(ctx context.Context, owner, repo string, runID int64, opts *github.ListOptions) (*github.ArtifactList, *github.Response, error)
	GetArtifact(ctx context.Context, owner, repo string, artifactID int64) (*github.Artifact, *github.Response, error)
}

// ArtifactFetcher wraps the artifact fetching logic with injectable services.
// Use NewArtifactFetcher to create an instance with custom services for testing.
type ArtifactFetcher struct {
	pullRequests PullRequestService
	actions      ActionsService
}

// NewArtifactFetcher creates an ArtifactFetcher with custom services.
// This is primarily used for testing with mock services.
func NewArtifactFetcher(prs PullRequestService, actions ActionsService) *ArtifactFetcher {
	defer perf.Track(nil, "github.NewArtifactFetcher")()

	return &ArtifactFetcher{pullRequests: prs, actions: actions}
}

// defaultArtifactFetcher returns a fetcher using the real GitHub client.
func defaultArtifactFetcher(ctx context.Context) *ArtifactFetcher {
	client := newGitHubClient(ctx)
	return &ArtifactFetcher{
		pullRequests: client.PullRequests,
		actions:      client.Actions,
	}
}

// defaultArtifactFetcherWithToken returns a fetcher using a GitHub client with an explicit token.
func defaultArtifactFetcherWithToken(ctx context.Context, token string) *ArtifactFetcher {
	client := newGitHubClientWithToken(ctx, token)
	return &ArtifactFetcher{
		pullRequests: client.PullRequests,
		actions:      client.Actions,
	}
}

// GetPRArtifactInfo retrieves build artifact information for a PR.
// This finds the latest successful workflow run for the PR and locates
// the artifact matching the current platform.
//
// Requires authentication - use GetGitHubTokenOrError() first.
func GetPRArtifactInfo(ctx context.Context, owner, repo string, prNumber int) (*PRArtifactInfo, error) {
	defer perf.Track(nil, "github.GetPRArtifactInfo")()

	fetcher := defaultArtifactFetcher(ctx)
	return fetcher.getPRArtifactInfo(ctx, owner, repo, prNumber)
}

// GetPRArtifactInfo retrieves build artifact info for a PR using custom services.
func (f *ArtifactFetcher) GetPRArtifactInfo(ctx context.Context, owner, repo string, prNumber int) (*PRArtifactInfo, error) {
	defer perf.Track(nil, "github.ArtifactFetcher.GetPRArtifactInfo")()

	return f.getPRArtifactInfo(ctx, owner, repo, prNumber)
}

// getPRArtifactInfo contains the core logic for retrieving PR artifact information.
func (f *ArtifactFetcher) getPRArtifactInfo(ctx context.Context, owner, repo string, prNumber int) (*PRArtifactInfo, error) {
	log.Debug("Fetching PR artifact info", logFieldOwner, owner, logFieldRepo, repo, "pr", prNumber)

	// Determine artifact name for current platform.
	artifactName, err := getArtifactNameForPlatform()
	if err != nil {
		return nil, err
	}

	// Step 1: Get PR info to find the head SHA.
	headSHA, err := getPRHeadSHA(ctx, f.pullRequests, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}

	log.Debug("Found PR head SHA", "sha", headSHA)

	// Step 2: Find the latest successful workflow run for this SHA.
	runInfo, err := findSuccessfulWorkflowRun(ctx, f.actions, owner, repo, headSHA)
	if err != nil {
		return nil, err
	}

	log.Debug("Found successful workflow run", "runID", runInfo.ID, "startedAt", runInfo.RunStartedAt)

	// Step 3: Find the artifact for the current platform.
	artifact, err := findArtifactByName(ctx, f.actions, owner, repo, runInfo.ID, artifactName)
	if err != nil {
		return nil, err
	}

	log.Debug("Found artifact", "name", artifact.GetName(), "size", artifact.GetSizeInBytes())

	return &PRArtifactInfo{
		PRNumber:     prNumber,
		HeadSHA:      headSHA,
		RunID:        runInfo.ID,
		ArtifactID:   artifact.GetID(),
		ArtifactName: artifact.GetName(),
		SizeInBytes:  artifact.GetSizeInBytes(),
		DownloadURL:  artifact.GetArchiveDownloadURL(),
		RunStartedAt: runInfo.RunStartedAt,
	}, nil
}

// GetArtifactDownloadURL returns the download URL for a specific artifact.
// The URL requires authentication to download.
func GetArtifactDownloadURL(ctx context.Context, owner, repo string, artifactID int64) (string, error) {
	defer perf.Track(nil, "github.GetArtifactDownloadURL")()

	fetcher := defaultArtifactFetcher(ctx)
	return fetcher.getArtifactDownloadURL(ctx, owner, repo, artifactID)
}

// GetArtifactDownloadURL returns the download URL using custom services.
func (f *ArtifactFetcher) GetArtifactDownloadURL(ctx context.Context, owner, repo string, artifactID int64) (string, error) {
	defer perf.Track(nil, "github.ArtifactFetcher.GetArtifactDownloadURL")()

	return f.getArtifactDownloadURL(ctx, owner, repo, artifactID)
}

// getArtifactDownloadURL contains the core logic for retrieving an artifact download URL.
func (f *ArtifactFetcher) getArtifactDownloadURL(ctx context.Context, owner, repo string, artifactID int64) (string, error) {
	// Get the artifact to retrieve its download URL.
	artifact, resp, err := f.actions.GetArtifact(ctx, owner, repo, artifactID)
	if err != nil {
		return "", handleGitHubAPIError(err, resp)
	}

	return artifact.GetArchiveDownloadURL(), nil
}

// getArtifactNameForPlatform returns the artifact name for the current OS/arch.
// Current CI builds:
//   - linux/amd64 -> build-artifacts-linux
//   - darwin/arm64 -> build-artifacts-macos
//   - windows/amd64 -> build-artifacts-windows
func getArtifactNameForPlatform() (string, error) {
	defer perf.Track(nil, "github.getArtifactNameForPlatform")()

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	switch goos {
	case "linux":
		if goarch == "amd64" {
			return "build-artifacts-linux", nil
		}
		return "", fmt.Errorf("%w: linux/%s (only linux/amd64 is built in CI)", ErrNoArtifactForPlatform, goarch)
	case "darwin":
		if goarch == "arm64" {
			return "build-artifacts-macos", nil
		}
		return "", fmt.Errorf("%w: darwin/%s (only darwin/arm64 is built in CI)", ErrNoArtifactForPlatform, goarch)
	case "windows":
		if goarch == "amd64" {
			return "build-artifacts-windows", nil
		}
		return "", fmt.Errorf("%w: windows/%s (only windows/amd64 is built in CI)", ErrNoArtifactForPlatform, goarch)
	default:
		return "", fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, goos, goarch)
	}
}

// getPRHeadSHA retrieves the head commit SHA for a pull request.
func getPRHeadSHA(ctx context.Context, prs PullRequestService, owner, repo string, prNumber int) (string, error) {
	defer perf.Track(nil, "github.getPRHeadSHA")()

	pr, resp, err := prs.Get(ctx, owner, repo, prNumber)
	if err != nil {
		if resp != nil && resp.StatusCode == statusNotFound {
			return "", fmt.Errorf("%w: #%d in %s/%s", ErrPRNotFound, prNumber, owner, repo)
		}
		return "", handleGitHubAPIError(err, resp)
	}

	if pr.Head == nil || pr.Head.SHA == nil {
		return "", fmt.Errorf("%w: PR #%d has no head SHA", ErrPRNotFound, prNumber)
	}

	return *pr.Head.SHA, nil
}

// findSuccessfulWorkflowRun finds the most recent successful workflow run for a commit SHA.
func findSuccessfulWorkflowRun(ctx context.Context, actions ActionsService, owner, repo, headSHA string) (*workflowRunInfo, error) {
	defer perf.Track(nil, "github.findSuccessfulWorkflowRun")()

	// List workflow runs for the commit SHA.
	opts := &github.ListWorkflowRunsOptions{
		HeadSHA: headSHA,
		Status:  "success",
		ListOptions: github.ListOptions{
			PerPage: perPage,
		},
	}

	runs, resp, err := actions.ListRepositoryWorkflowRuns(ctx, owner, repo, opts)
	if err != nil {
		return nil, handleGitHubAPIError(err, resp)
	}

	// Find the workflow run with the correct name ("Tests").
	for _, run := range runs.WorkflowRuns {
		if run.GetName() == workflowName && run.GetConclusion() == "success" {
			return &workflowRunInfo{
				ID:           run.GetID(),
				RunStartedAt: run.GetRunStartedAt().Time,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: no successful '%s' workflow run for SHA %s", ErrNoWorkflowRunFound, workflowName, headSHA)
}

// findArtifactByName finds an artifact by name within a workflow run.
//
//nolint:revive // All parameters are necessary for this GitHub API function.
func findArtifactByName(ctx context.Context, actions ActionsService, owner, repo string, runID int64, artifactName string) (*github.Artifact, error) {
	defer perf.Track(nil, "github.findArtifactByName")()

	opts := &github.ListOptions{
		PerPage: perPage,
	}

	artifacts, resp, err := actions.ListWorkflowRunArtifacts(ctx, owner, repo, runID, opts)
	if err != nil {
		return nil, handleGitHubAPIError(err, resp)
	}

	for _, artifact := range artifacts.Artifacts {
		if artifact.GetName() == artifactName {
			return artifact, nil
		}
	}

	// Build list of available artifacts for error message.
	available := make([]string, 0, len(artifacts.Artifacts))
	for _, a := range artifacts.Artifacts {
		available = append(available, a.GetName())
	}

	return nil, fmt.Errorf("%w: '%s' not found in workflow run %d (available: %v)",
		ErrNoArtifactFound, artifactName, runID, available)
}

// SupportedPRPlatforms returns a list of platforms supported by PR artifact downloads.
func SupportedPRPlatforms() []string {
	defer perf.Track(nil, "github.SupportedPRPlatforms")()

	return []string{
		"linux/amd64",
		"darwin/arm64",
		"windows/amd64",
	}
}

// GetPRHeadSHA retrieves the current head commit SHA for a pull request.
// This is used for cache validation to check if the PR has new commits.
// The token parameter is required for API authentication.
func GetPRHeadSHA(ctx context.Context, owner, repo string, prNumber int, token string) (string, error) {
	defer perf.Track(nil, "github.GetPRHeadSHA")()

	fetcher := defaultArtifactFetcherWithToken(ctx, token)
	return getPRHeadSHA(ctx, fetcher.pullRequests, owner, repo, prNumber)
}

// GetPRHeadSHA retrieves the head SHA for a PR using custom services.
func (f *ArtifactFetcher) GetPRHeadSHA(ctx context.Context, owner, repo string, prNumber int) (string, error) {
	defer perf.Track(nil, "github.ArtifactFetcher.GetPRHeadSHA")()

	return getPRHeadSHA(ctx, f.pullRequests, owner, repo, prNumber)
}

// GetSHAArtifactInfo retrieves build artifact information for a commit SHA.
// This finds the latest successful workflow run for the SHA and locates
// the artifact matching the current platform.
//
// Requires authentication - use GetGitHubTokenOrError() first.
func GetSHAArtifactInfo(ctx context.Context, owner, repo, sha string) (*SHAArtifactInfo, error) {
	defer perf.Track(nil, "github.GetSHAArtifactInfo")()

	fetcher := defaultArtifactFetcher(ctx)
	return fetcher.getSHAArtifactInfo(ctx, owner, repo, sha)
}

// GetSHAArtifactInfo retrieves build artifact info for a SHA using custom services.
func (f *ArtifactFetcher) GetSHAArtifactInfo(ctx context.Context, owner, repo, sha string) (*SHAArtifactInfo, error) {
	defer perf.Track(nil, "github.ArtifactFetcher.GetSHAArtifactInfo")()

	return f.getSHAArtifactInfo(ctx, owner, repo, sha)
}

// getSHAArtifactInfo contains the core logic for retrieving SHA artifact information.
func (f *ArtifactFetcher) getSHAArtifactInfo(ctx context.Context, owner, repo, sha string) (*SHAArtifactInfo, error) {
	log.Debug("Fetching SHA artifact info", logFieldOwner, owner, logFieldRepo, repo, "sha", sha)

	// Determine artifact name for current platform.
	artifactName, err := getArtifactNameForPlatform()
	if err != nil {
		return nil, err
	}

	// Find the latest successful workflow run for this SHA.
	runInfo, err := findSuccessfulWorkflowRun(ctx, f.actions, owner, repo, sha)
	if err != nil {
		return nil, err
	}

	log.Debug("Found successful workflow run", "runID", runInfo.ID, "startedAt", runInfo.RunStartedAt)

	// Find the artifact for the current platform.
	artifact, err := findArtifactByName(ctx, f.actions, owner, repo, runInfo.ID, artifactName)
	if err != nil {
		return nil, err
	}

	log.Debug("Found artifact", "name", artifact.GetName(), "size", artifact.GetSizeInBytes())

	return &SHAArtifactInfo{
		HeadSHA:      sha,
		RunID:        runInfo.ID,
		ArtifactID:   artifact.GetID(),
		ArtifactName: artifact.GetName(),
		SizeInBytes:  artifact.GetSizeInBytes(),
		DownloadURL:  artifact.GetArchiveDownloadURL(),
		RunStartedAt: runInfo.RunStartedAt,
	}, nil
}

// IsNotFoundError checks if the error is a "not found" type error.
func IsNotFoundError(err error) bool {
	defer perf.Track(nil, "github.IsNotFoundError")()

	return errors.Is(err, ErrPRNotFound)
}

// IsNoWorkflowError checks if the error is a "no workflow run" error.
func IsNoWorkflowError(err error) bool {
	defer perf.Track(nil, "github.IsNoWorkflowError")()

	return errors.Is(err, ErrNoWorkflowRunFound)
}

// IsNoArtifactError checks if the error is a "no artifact" error.
func IsNoArtifactError(err error) bool {
	defer perf.Track(nil, "github.IsNoArtifactError")()

	return errors.Is(err, ErrNoArtifactFound)
}

// IsPlatformError checks if the error is a platform-related error.
func IsPlatformError(err error) bool {
	defer perf.Track(nil, "github.IsPlatformError")()

	return errors.Is(err, ErrNoArtifactForPlatform) || errors.Is(err, ErrUnsupportedPlatform)
}
