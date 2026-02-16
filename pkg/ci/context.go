package ci

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
