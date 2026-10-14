package ci

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// commentMarkerPrefix and commentMarkerSuffix wrap a comment key into the HTML marker
// providers use to find an existing comment.
const (
	commentMarkerPrefix = "<!-- atmos:ci:"
	commentMarkerSuffix = " -->"
)

// commentPlan is a validated comment request: the behavior and target are resolved and the marker
// is built, but the run context is not yet consulted.
type commentPlan struct {
	body     string
	marker   string
	behavior CommentBehavior
	target   CommentTarget
	pr       int
}

// planComment validates a comment request. Argument errors surface here, before any gate, so a
// mistake is visible on a laptop and in a held fork run alike.
func planComment(req CommentRequest) (commentPlan, error) {
	target, err := provider.NormalizeTarget(req.Target)
	if err != nil {
		return commentPlan{}, err
	}
	plan := commentPlan{body: req.Body, behavior: CommentBehaviorCreate, target: target, pr: req.PR}
	if req.Key == "" {
		if req.Behavior == CommentBehaviorUpdate {
			return commentPlan{}, errUtils.Build(errUtils.ErrCICommentKeyRequired).
				WithHint("Name the comment with a key so it can be found again, or use the create behavior").
				Err()
		}
		return plan, nil
	}
	plan.marker = commentMarkerPrefix + req.Key + commentMarkerSuffix
	plan.body = plan.marker + "\n" + req.Body
	plan.behavior, err = provider.NormalizeBehavior(req.Behavior)
	if err != nil {
		return commentPlan{}, err
	}
	return plan, nil
}

// resolveTarget decides where a comment lands and the pull request number when it is a pull
// request comment. The auto target picks the pull request when one is known, otherwise the commit.
func (plan commentPlan) resolveTarget(ciCtx *Context) (CommentTarget, int) {
	prNumber := plan.pr
	if prNumber == 0 && ciCtx.PullRequest != nil {
		prNumber = ciCtx.PullRequest.Number
	}
	if plan.target == CommentTargetAuto {
		if prNumber > 0 {
			return CommentTargetPR, prNumber
		}
		return CommentTargetCommit, 0
	}
	return plan.target, prNumber
}

func (r *reporter) Comment(ctx context.Context, req CommentRequest) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Comment")()

	plan, err := planComment(req)
	if err != nil {
		return Receipt{}, err
	}
	g, err := r.gated(FeatureComments, CommentsEnabled)
	if err != nil || g.provider == nil {
		return g.receipt, err
	}
	p, rc, ciCtx := g.provider, g.receipt, g.ciCtx

	target, prNumber := plan.resolveTarget(ciCtx)
	// A missing pull request or commit is an argument problem, reported as such rather than as a failed post.
	if err := requireTargetKnown(target, prNumber, ciCtx, rc.Local); err != nil {
		return rc, err
	}
	var comment *Comment
	if target == CommentTargetCommit {
		p, rc = r.commitCommenterFor(p, rc)
		comment, err = postCommitComment(ctx, p, plan, ciCtx)
	} else {
		comment, err = postPullRequestComment(ctx, p, plan, ciCtx, prNumber)
	}
	if err != nil {
		return rc, wrapErr(errUtils.ErrCICommentPostFailed, err)
	}
	if comment != nil {
		comment.Target = target
	}
	rc.Comment = comment
	return rc, nil
}

// commitCommenterFor returns the provider that can post commit comments: p when it can, otherwise
// the local provider, mirroring how routeTo falls back for other optional capabilities.
func (r *reporter) commitCommenterFor(p provider.Provider, rc Receipt) (provider.Provider, Receipt) {
	if _, ok := p.(provider.CommitCommenter); ok || rc.Local {
		return p, rc
	}
	if l := r.local(); l != nil {
		if _, ok := l.(provider.CommitCommenter); ok {
			log.Debug("CI provider lacks commit comments, using local rendering", "provider", p.Name())
			return l, r.localReceipt(l, rc.Gate)
		}
	}
	return p, rc
}

// requireTargetKnown fails when a platform write names a pull request or commit that is unknown.
// A local rendering accepts empty values, because a laptop run has neither.
func requireTargetKnown(target CommentTarget, prNumber int, ciCtx *Context, local bool) error {
	if local {
		return nil
	}
	switch {
	case target == CommentTargetCommit && ciCtx.SHA == "":
		return errUtils.Build(errUtils.ErrCICommitUnknown).
			WithHint("Run in a checkout of the commit to comment on, or comment on a pull request").
			Err()
	case target == CommentTargetPR && prNumber <= 0:
		return errUtils.Build(errUtils.ErrCIPullRequestUnknown).
			WithHint("Pass the pull request number or run in a pull request context").
			Err()
	}
	return nil
}

// postPullRequestComment posts a pull request comment.
func postPullRequestComment(ctx context.Context, p provider.Provider, plan commentPlan, ciCtx *Context, prNumber int) (*Comment, error) {
	return p.PostComment(ctx, &PostCommentOptions{
		Owner:    ciCtx.RepoOwner,
		Repo:     ciCtx.RepoName,
		PRNumber: prNumber,
		Marker:   plan.marker,
		Body:     plan.body,
		Behavior: plan.behavior,
	})
}

// postCommitComment posts a comment on the run's commit.
func postCommitComment(ctx context.Context, p provider.Provider, plan commentPlan, ciCtx *Context) (*Comment, error) {
	commenter, ok := p.(provider.CommitCommenter)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrCIOperationNotSupported).
			WithExplanation("The CI provider cannot comment on a commit").
			WithContext("provider", p.Name()).
			Err()
	}
	return commenter.PostCommitComment(ctx, &PostCommitCommentOptions{
		Owner:    ciCtx.RepoOwner,
		Repo:     ciCtx.RepoName,
		SHA:      ciCtx.SHA,
		Marker:   plan.marker,
		Body:     plan.body,
		Behavior: plan.behavior,
	})
}
