package ci

import "github.com/cloudposse/atmos/pkg/ci/internal/provider"

// Context is the public alias for CI run metadata supplied by a provider.
// Consumers outside pkg/ci (e.g. cmd/git for CI checkout replacement) use
// this alias; the underlying type lives in the internal provider package.
type Context = provider.Context

// PRInfo is the public alias for pull request metadata.
type PRInfo = provider.PRInfo

// Comment, CommentBehavior, and the check-run types are re-exported from the
// internal provider package so callers outside pkg/ci can post comments and
// manage check runs without importing an internal package. They are type
// aliases, so the internal types remain the single source of truth.
type (
	// Comment is a PR/MR comment returned by PostComment.
	Comment = provider.Comment
	// CommentBehavior controls how PostComment reconciles against existing comments.
	CommentBehavior = provider.CommentBehavior
	// CommentTarget selects whether a comment lands on the pull request or on the commit.
	CommentTarget = provider.CommentTarget
	// PostCommitCommentOptions contains options for posting or upserting a commit comment.
	PostCommitCommentOptions = provider.PostCommitCommentOptions
	// PostCommentOptions contains options for posting or upserting a PR/MR comment.
	PostCommentOptions = provider.PostCommentOptions
	// CheckRun is a provider check run (status check).
	CheckRun = provider.CheckRun
	// CheckRunState is the state of a check run.
	CheckRunState = provider.CheckRunState
	// CreateCheckRunOptions contains options for creating a new check run.
	CreateCheckRunOptions = provider.CreateCheckRunOptions
	// UpdateCheckRunOptions contains options for updating an existing check run.
	UpdateCheckRunOptions = provider.UpdateCheckRunOptions
)

// Re-exported comment behaviors (see provider.CommentBehavior).
const (
	CommentBehaviorCreate = provider.CommentBehaviorCreate
	CommentBehaviorUpdate = provider.CommentBehaviorUpdate
	CommentBehaviorUpsert = provider.CommentBehaviorUpsert
)

// Re-exported comment targets (see provider.CommentTarget).
const (
	CommentTargetAuto   = provider.CommentTargetAuto
	CommentTargetPR     = provider.CommentTargetPR
	CommentTargetCommit = provider.CommentTargetCommit
)

// Re-exported check run states (see provider.CheckRunState).
const (
	CheckRunStatePending    = provider.CheckRunStatePending
	CheckRunStateInProgress = provider.CheckRunStateInProgress
	CheckRunStateSuccess    = provider.CheckRunStateSuccess
	CheckRunStateFailure    = provider.CheckRunStateFailure
	CheckRunStateError      = provider.CheckRunStateError
	CheckRunStateCancelled  = provider.CheckRunStateCancelled
)
