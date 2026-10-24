package provider

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

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

// CommentTarget selects where a comment lands: on the pull request or on the commit.
type CommentTarget string

const (
	// CommentTargetAuto posts to the pull request when one is known and to the commit otherwise.
	// An empty target means auto.
	CommentTargetAuto CommentTarget = "auto"

	// CommentTargetPR posts to the pull request and fails when none is known.
	CommentTargetPR CommentTarget = "pr"

	// CommentTargetCommit posts a commit comment on the run's commit SHA.
	CommentTargetCommit CommentTarget = "commit"
)

// NormalizeTarget resolves the configured target. An empty value means auto; any other value
// must be one of the declared CommentTarget constants.
func NormalizeTarget(t CommentTarget) (CommentTarget, error) {
	defer perf.Track(nil, "provider.NormalizeTarget")()

	switch t {
	case "":
		return CommentTargetAuto, nil
	case CommentTargetAuto, CommentTargetPR, CommentTargetCommit:
		return t, nil
	default:
		return "", errUtils.Build(errUtils.ErrCICommentTargetInvalid).
			WithExplanation("The comment target must be one of: auto, pr, commit").
			WithContext("target", string(t)).
			Err()
	}
}

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

	// Target is where the comment landed: CommentTargetPR or CommentTargetCommit.
	// The reporter sets it; providers need not.
	Target CommentTarget
}

// PostCommitCommentOptions contains options for posting or upserting a comment on a commit.
type PostCommitCommentOptions struct {
	// Owner is the repository owner (GitHub) or namespace (GitLab).
	Owner string

	// Repo is the repository name.
	Repo string

	// SHA is the commit the comment is attached to.
	SHA string

	// Marker is the HTML/Markdown marker used to find an existing comment on repeat runs.
	// It must appear in Body.
	Marker string

	// Body is the full comment body (including Marker).
	Body string

	// Behavior controls create/update/upsert semantics. Empty defaults to
	// CommentBehaviorUpsert.
	Behavior CommentBehavior
}

// ValidatePostCommitCommentOptions rejects nil or incomplete option structs and enforces the
// marker-in-body invariant, as ValidatePostCommentOptions does for pull request comments.
func ValidatePostCommitCommentOptions(opts *PostCommitCommentOptions) error {
	defer perf.Track(nil, "provider.ValidatePostCommitCommentOptions")()

	if opts == nil || opts.Owner == "" || opts.Repo == "" || opts.SHA == "" {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Owner, Repo, and SHA are required to post a commit comment").
			Err()
	}
	if opts.Marker != "" && !strings.Contains(opts.Body, opts.Marker) {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Marker must appear in Body so future runs can find and update this comment; without it, upserts will create duplicates").
			WithContext("marker", opts.Marker).
			Err()
	}
	return nil
}

// ValidatePostCommentOptions rejects nil or incomplete option structs, and
// enforces the marker-in-body invariant so repeat runs can reliably reconcile
// against the same comment. An upsert that writes a body without its marker
// would leave a comment that future runs cannot match — breaking idempotency
// and causing duplicate comments on subsequent plans.
func ValidatePostCommentOptions(opts *PostCommentOptions) error {
	defer perf.Track(nil, "provider.ValidatePostCommentOptions")()

	if opts == nil || opts.Owner == "" || opts.Repo == "" || opts.PRNumber <= 0 {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Owner, Repo, and PRNumber are required to post a PR comment").
			Err()
	}
	if opts.Marker != "" && !strings.Contains(opts.Body, opts.Marker) {
		return errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("Marker must appear in Body so future runs can find and update this comment; without it, upserts will create duplicates").
			WithContext("marker", opts.Marker).
			Err()
	}
	return nil
}

// NormalizeBehavior resolves the configured behavior. An empty value defaults
// to upsert; any other value must be one of the declared CommentBehavior
// constants. Unknown values fail fast so typos in ci.comments.behavior surface
// immediately rather than silently behaving as upsert.
func NormalizeBehavior(b CommentBehavior) (CommentBehavior, error) {
	defer perf.Track(nil, "provider.NormalizeBehavior")()

	switch b {
	case "":
		return CommentBehaviorUpsert, nil
	case CommentBehaviorCreate, CommentBehaviorUpdate, CommentBehaviorUpsert:
		return b, nil
	default:
		return "", errUtils.Build(errUtils.ErrCICommentPostFailed).
			WithExplanation("ci.comments.behavior must be one of: create, update, upsert").
			WithContext("behavior", string(b)).
			Err()
	}
}
