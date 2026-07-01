// Package provider defines the CI/CD provider interface and related types.
package provider

import (
	"context"
	"io"

	"github.com/cloudposse/atmos/pkg/ci/cache"
)

// BaseResolution contains the resolved base commit for affected detection.
type BaseResolution struct {
	// Ref is a git reference (branch/tag). Mutually exclusive with SHA.
	Ref string

	// SHA is a git commit hash. Mutually exclusive with Ref.
	SHA string

	// HeadSHA is the PR head commit SHA for upload correlation with Atmos Pro.
	// Populated for pull_request events from event.pull_request.head.sha.
	// Empty for non-PR events (push, merge_group, etc.).
	HeadSHA string

	// TargetBranch is the PR target branch name (e.g., "main") when known.
	// Used by callers to recover from missing local refs by running a
	// targeted git fetch. Empty when the event has no notion of a target
	// branch (e.g., push events on the default branch).
	TargetBranch string

	// Source describes where the base was resolved from (for logging).
	Source string

	// EventType describes the CI event (e.g., "pull_request", "push").
	EventType string
}

// Provider represents a CI/CD provider (GitHub Actions, GitLab CI, etc.).
type Provider interface {
	// Name returns the provider name (e.g., "github-actions", "generic").
	Name() string

	// Detect returns true if this provider is active in the current environment.
	Detect() bool

	// Context returns CI metadata (run ID, PR info, etc.).
	Context() (*Context, error)

	// GetStatus returns PR/commit status for the current branch.
	GetStatus(ctx context.Context, opts StatusOptions) (*Status, error)

	// CreateCheckRun creates a new check run on a commit (like Atlantis status checks).
	CreateCheckRun(ctx context.Context, opts *CreateCheckRunOptions) (*CheckRun, error)

	// UpdateCheckRun updates an existing check run.
	UpdateCheckRun(ctx context.Context, opts *UpdateCheckRunOptions) (*CheckRun, error)

	// PostComment posts or upserts a PR/MR comment. Providers that do not
	// support comments should return errUtils.ErrCIOperationNotSupported.
	// Implementations use the marker string to find and update existing
	// comments on repeat runs (upsert); callers embed the marker in Body.
	PostComment(ctx context.Context, opts *PostCommentOptions) (*Comment, error)

	// OutputWriter returns a writer for CI outputs ($GITHUB_OUTPUT, etc.).
	OutputWriter() OutputWriter

	// ResolveBase returns the base commit for affected detection.
	// Returns nil if the provider cannot determine the base.
	ResolveBase() (*BaseResolution, error)
}

// DebugModeDetector is an optional capability for providers that expose a
// "debug mode" signal set at the runner / step / job level (for example,
// GitHub Actions' ACTIONS_RUNNER_DEBUG and ACTIONS_STEP_DEBUG). Providers
// implement this when their platform has a documented way for users to opt
// into verbose diagnostic logging for a run.
type DebugModeDetector interface {
	// IsDebugMode reports whether the current run has debug logging enabled
	// at the CI provider level. Callers use this to auto-promote their own
	// log level.
	IsDebugMode() bool
}

// LogGrouper is an optional capability for CI providers whose log viewer
// supports collapsible, named groups (for example GitHub Actions' workflow
// commands `::group::<name>` / `::endgroup::`). Providers implement this when
// their platform documents a way to fold a region of the run log under a
// label. Callers (the custom-command and workflow step runners) use it to wrap
// each step's output in its own collapsible group. Providers without log
// grouping simply do not implement this interface, and grouping becomes a
// no-op for them.
type LogGrouper interface {
	// StartGroup writes a group-start marker named `name` to w.
	StartGroup(w io.Writer, name string)

	// EndGroup writes the matching group-end marker to w.
	EndGroup(w io.Writer)
}

// CacheProvider is an optional capability for CI providers that expose a remote
// build cache (for example, the GitHub Actions cache). Providers implement this
// when their platform offers a documented cache store reachable from within a
// run. Cache() returns errUtils.ErrCacheUnavailable when the provider is active
// but the cache cannot be reached in the current environment (e.g. the runtime
// cache token is absent).
type CacheProvider interface {
	// Cache returns the provider's cache backend.
	Cache() (cache.Backend, error)
}

// OutputWriter writes CI outputs (environment variables, job summaries, etc.).
type OutputWriter interface {
	// WriteOutput writes a key-value pair to CI outputs (e.g., $GITHUB_OUTPUT).
	WriteOutput(key, value string) error

	// WriteSummary writes content to the job summary (e.g., $GITHUB_STEP_SUMMARY).
	WriteSummary(content string) error
}

// Context contains CI run metadata.
type Context struct {
	// Provider is the name of the CI provider (e.g., "github-actions").
	Provider string

	// RunID is the unique identifier for this CI run.
	RunID string

	// RunNumber is the run number (increments per workflow).
	RunNumber int

	// Workflow is the name of the workflow.
	Workflow string

	// Job is the name of the current job.
	Job string

	// Actor is the user or app that triggered the workflow.
	Actor string

	// EventName is the event that triggered the workflow (e.g., "push", "pull_request").
	EventName string

	// Ref is the git ref (e.g., "refs/heads/main").
	Ref string

	// Branch is the branch name (e.g., "main", "feature/foo").
	Branch string

	// SHA is the git commit SHA.
	SHA string

	// Repository is the full repository name (e.g., "owner/repo").
	Repository string

	// RepoOwner is the repository owner.
	RepoOwner string

	// RepoName is the repository name.
	RepoName string

	// ServerURL is the base URL of the SCM host running this CI
	// (e.g., "https://github.com" or a GitHub Enterprise URL).
	// Empty when the provider cannot determine it.
	ServerURL string

	// CloneURL is the URL to clone the current repository, used by
	// `atmos git clone` for CI checkout replacement. Each provider
	// constructs it from its own metadata; empty when the provider
	// cannot determine it.
	CloneURL string

	// PullRequest contains PR info if this is a pull request event.
	PullRequest *PRInfo
}

// PRInfo contains pull request metadata.
type PRInfo struct {
	// Number is the PR number.
	Number int

	// HeadRef is the source branch.
	HeadRef string

	// BaseRef is the target branch.
	BaseRef string

	// URL is the PR URL.
	URL string
}
