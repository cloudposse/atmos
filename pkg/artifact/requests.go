package artifact

// PublishRequest publishes a local source to one repository.
type PublishRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Source is the local source file or directory.
	Source string

	// Name is the explicit artifact name. Precedence: explicit, producer metadata, source basename.
	Name string

	// Version is the explicit version. Precedence: explicit, producer metadata; otherwise fail.
	Version string

	// Kind is the artifact kind; empty means [KindFiles].
	Kind string

	// Metadata is the explicit user metadata (description and labels).
	Metadata Metadata

	// Readme overrides the README source path, relative to the source directory.
	Readme string

	// Changelog overrides the CHANGELOG source path, relative to the source directory.
	Changelog string

	// Force permits replacing an existing different artifact where the repository allows it.
	Force bool

	// DryRun validates and plans without writing.
	DryRun bool
}

// PullRequest materializes an artifact into a local directory.
type PullRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref

	// Destination is the output directory.
	Destination string

	// Force permits overwriting existing destination content.
	Force bool
}

// CopyRequest copies an artifact between two configured repositories.
type CopyRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Ref selects the artifact in the source repository.
	Ref Ref

	// From is the source repository alias.
	From string

	// To is the destination repository alias.
	To string

	// Force permits replacing a conflicting destination. It never bypasses verification.
	Force bool
}

// InspectRequest inspects an artifact without downloading its payload.
type InspectRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref
}

// ListRequest lists artifacts. List and search intentionally default to all configured repositories.
type ListRequest struct {
	// Repositories restricts the listing; empty means all configured repositories.
	Repositories []string

	// Name restricts the listing to versions of one exact name.
	Name string

	// Labels keep only entries whose labels match every selector exactly.
	Labels []Label

	// Refresh bypasses cached discovery snapshots.
	Refresh bool
}

// SearchRequest searches artifacts by text.
type SearchRequest struct {
	// Repositories restricts the search; empty means all configured repositories.
	Repositories []string

	// Query is the search text matched against names, kinds, descriptions, and labels.
	Query string

	// Labels keep only entries whose labels match every selector exactly.
	Labels []Label

	// Refresh bypasses cached discovery snapshots.
	Refresh bool
}

// DeleteRequest deletes an artifact from one repository.
type DeleteRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref
}

// ShowDocumentRequest reads one package document.
type ShowDocumentRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref

	// Role is readme or changelog.
	Role DocumentRole
}

// SignRequest adds signature evidence to an immutable subject.
type SignRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref must be a digest reference; sign accepts digest references only.
	Ref Ref
}

// VerifyRequest verifies an artifact's signature evidence against the effective policy.
type VerifyRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref
}

// PinRequest creates an immutable reference for consumer lockfiles.
type PinRequest struct {
	// Execution carries the wrapper-owned operation identity and retry budget.
	Execution Execution

	// Repository is the required configured repository alias.
	Repository string

	// Ref selects the artifact.
	Ref Ref
}
