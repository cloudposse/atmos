package provider

// CommentBehavior controls how PostComment reconciles an incoming comment
// against existing comments on the same PR/MR.
type CommentBehavior string

const (
	// CommentBehaviorCreate always creates a new comment, even if a comment
	// with the same marker already exists.
	CommentBehaviorCreate CommentBehavior = "create"

	// CommentBehaviorUpdate updates an existing comment matched by marker.
	// Returns ErrCICommentNotFound when no matching comment exists.
	CommentBehaviorUpdate CommentBehavior = "update"

	// CommentBehaviorUpsert updates an existing comment matched by marker,
	// creating a new one when none matches. This is the default.
	CommentBehaviorUpsert CommentBehavior = "upsert"
)

// PostCommentOptions contains options for posting or upserting a PR/MR comment.
type PostCommentOptions struct {
	// Owner is the repository owner (GitHub) or namespace (GitLab).
	Owner string

	// Repo is the repository name.
	Repo string

	// PRNumber is the pull/merge request number.
	PRNumber int

	// Marker is an HTML/Markdown marker string used to find existing comments
	// on repeat runs. It must appear in Body. Typical value:
	//   "<!-- atmos:ci:plan:<component>:<stack> -->".
	Marker string

	// Body is the full comment body (including Marker).
	Body string

	// Behavior controls create/update/upsert semantics. Empty defaults to
	// CommentBehaviorUpsert.
	Behavior CommentBehavior
}

// Comment represents a PR/MR comment returned by PostComment.
type Comment struct {
	// ID is the provider-specific comment ID.
	ID int64

	// URL is the HTML URL of the comment (if known).
	URL string

	// Body is the final body that was written.
	Body string

	// Created indicates whether a new comment was created (true) or an
	// existing one was updated (false).
	Created bool
}
