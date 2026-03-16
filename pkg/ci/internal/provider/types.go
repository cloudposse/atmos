// Package provider defines the CI/CD provider interface and related types.
package provider

import "context"

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

	// OutputWriter returns a writer for CI outputs ($GITHUB_OUTPUT, etc.).
	OutputWriter() OutputWriter
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
