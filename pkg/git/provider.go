package git

import (
	"context"
)

// SigningMode controls commit signing behavior.
type SigningMode string

const (
	// SigningAuto passes no signing flag; Git config decides.
	SigningAuto SigningMode = "auto"
	// SigningAlways passes -S to git commit.
	SigningAlways SigningMode = "always"
	// SigningNever passes --no-gpg-sign to git commit.
	SigningNever SigningMode = "never"
)

// Author is the commit author/committer identity injected per invocation
// (CI runners typically have no user.name/user.email configured).
type Author struct {
	Name  string
	Email string
}

// RepoContext carries the common inputs every repository operation needs.
type RepoContext struct {
	// Workdir is the repository worktree directory.
	Workdir string
	// Remote name; empty means "origin".
	Remote string
	// Branch to operate on; empty means the current/default branch.
	Branch string
	// Env is the fully composed subprocess environment (process env plus
	// identity environment). Nil means the current process environment.
	Env []string
}

// CloneOptions configures Clone. Clone is defined as reconcile: clone when the
// workdir is absent, otherwise fetch and fast-forward to the expected ref.
type CloneOptions struct {
	RepoContext
	// URI is the remote repository URI.
	URI string
	// Depth is the shallow-clone depth; 0 means full history.
	Depth int
	// Filter is an optional partial-clone filter spec (e.g. "blob:none").
	Filter string
	// SingleBranch limits the clone to the configured branch.
	SingleBranch bool
	// Submodules enables submodule initialization.
	Submodules bool
	// ExtraArgs are native git arguments (from `--` on the command line)
	// appended verbatim to the `git clone` invocation by the cli provider.
	// They apply only to a fresh clone, not when reconciling an existing
	// workdir (where no `git clone` runs).
	ExtraArgs []string
}

// PullOptions configures Pull. Pull is always fast-forward-only.
type PullOptions struct {
	RepoContext
	// ExtraArgs are native git arguments (from `--` on the command line)
	// appended verbatim to the `git pull --ff-only` invocation by the cli
	// provider.
	ExtraArgs []string
}

// StatusOptions configures Status.
type StatusOptions struct {
	RepoContext
	// Paths limits status to the given repo-relative paths.
	Paths []string
}

// StatusEntry is one porcelain status entry.
type StatusEntry struct {
	// Code is the two-character porcelain status code (e.g. " M", "??").
	Code string
	// Path is the repo-relative path.
	Path string
}

// StatusResult reports worktree state.
type StatusResult struct {
	// Clean is true when there are no changes (within Paths, when given).
	Clean bool
	// Entries lists the porcelain entries.
	Entries []StatusEntry
}

// DiffOptions configures Diff.
type DiffOptions struct {
	RepoContext
	// Paths limits the diff to the given repo-relative paths.
	Paths []string
}

// DiffResult reports differences between the worktree and HEAD.
type DiffResult struct {
	// HasChanges is true when tracked or untracked changes exist.
	HasChanges bool
	// Output is the unified diff of tracked changes.
	Output string
	// Untracked lists untracked files not represented in Output.
	Untracked []string
}

// CommitOptions configures Commit.
type CommitOptions struct {
	RepoContext
	// Message is the commit message (trailers are appended separately).
	Message string
	// Paths scopes staging to the given repo-relative paths. When set, dirty
	// files outside these paths fail the commit (ErrGitDirtyUnmanagedFiles).
	// When empty, only already-staged changes are committed.
	Paths []string
	// Signing selects the signing mode; empty means SigningAuto.
	Signing SigningMode
	// Author overrides the author/committer identity when non-nil.
	Author *Author
	// Trailers are appended to the message as "Key: value" trailer lines
	// (provenance: Atmos-Stack, Atmos-Component, Atmos-Source-SHA).
	Trailers map[string]string
}

// CommitResult reports the outcome of Commit. A no-op commit is not an error:
// it returns Committed=false with a nil error.
type CommitResult struct {
	Committed bool
	SHA       string
}

// InitOptions configures Init. Init creates a new repository workdir from
// scratch — the inverse of Clone, for GitOps repositories whose remote has no
// content yet.
type InitOptions struct {
	RepoContext
	// URI is the configured remote repository URI, registered as the
	// configured remote (default "origin") so commit/push work immediately.
	URI string
	// FromURI optionally seeds the new repository's content from another
	// repository (a template or a repository being migrated).
	FromURI string
	// KeepHistory preserves FromURI's full history and keeps the source
	// reachable as an additional remote ("upstream") so future updates can be
	// pulled. When false (the default), the source content is imported with a
	// single fresh initial commit and no link to the source remains.
	KeepHistory bool
	// Signing selects the signing mode for the fresh initial commit created
	// when FromURI is set without KeepHistory; empty means SigningAuto.
	Signing SigningMode
	// Author overrides the author/committer identity for the fresh initial
	// commit when non-nil.
	Author *Author
	// ExtraArgs are native git arguments (from `--` on the command line)
	// appended verbatim to the primary git invocation by the cli provider:
	// `git init` for an empty init, or the `git clone` of FromURI.
	ExtraArgs []string
	// Force re-initializes in place when the workdir already exists instead of
	// refusing. It is non-destructive: `git init` is idempotent and the
	// configured remote is updated rather than duplicated. Force does not apply
	// to seeded (FromURI) modes, whose clone requires an empty target.
	Force bool
}

// PushOptions configures Push.
type PushOptions struct {
	RepoContext
	// Retries bounds the rebase-and-retry loop on non-fast-forward rejection.
	Retries int
	// ExtraArgs are native git arguments (from `--` on the command line)
	// appended verbatim to each `git push` invocation by the cli provider
	// (not to the rebase recovery between retries).
	ExtraArgs []string
}

// Provider is a pluggable Git execution backend. The "cli" provider shells out
// to the git CLI (the only v1 implementation); a future "github" provider may
// use host APIs for capabilities like pull-request publishing.
type Provider interface {
	Init(ctx context.Context, opts *InitOptions) error
	Clone(ctx context.Context, opts *CloneOptions) error
	Pull(ctx context.Context, opts *PullOptions) error
	Status(ctx context.Context, opts *StatusOptions) (*StatusResult, error)
	Diff(ctx context.Context, opts *DiffOptions) (*DiffResult, error)
	Commit(ctx context.Context, opts *CommitOptions) (*CommitResult, error)
	Push(ctx context.Context, opts *PushOptions) error
}
