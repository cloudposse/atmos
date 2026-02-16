// Package ci provides CI/CD provider abstractions and integrations.
package ci

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
