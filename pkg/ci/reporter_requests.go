package ci

import (
	"context"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// commentMarkerPrefix and commentMarkerSuffix wrap a comment key into the HTML marker
// providers use to find an existing comment.
const (
	commentMarkerPrefix = "<!-- atmos:ci:"
	commentMarkerSuffix = " -->"
)

// terminalConclusion maps a final check run state to its conclusion. It mirrors the
// terraform plugin's mapping (success or failure), with cancelled kept distinct.
func terminalConclusion(state CheckRunState) (string, bool) {
	switch state {
	case CheckRunStateSuccess:
		return "success", true
	case CheckRunStateFailure, CheckRunStateError:
		return "failure", true
	case CheckRunStateCancelled:
		return "cancelled", true
	default:
		return "", false
	}
}

// requestContext returns the CI context for a request. A missing context is tolerated
// when the request renders locally, because the local rendering accepts empty values.
func (r *reporter) requestContext(rc Receipt) (*Context, error) {
	c, err := r.Context()
	if err != nil {
		if rc.Local {
			log.Debug("CI context unavailable, rendering locally without it", "error", err)
			return &Context{}, nil
		}
		return nil, err
	}
	if c == nil {
		return &Context{}, nil
	}
	return c, nil
}

// gatedTarget is the resolved destination for a write that needs repository identity.
type gatedTarget struct {
	provider provider.Provider
	receipt  Receipt
	ciCtx    *Context
}

// gated resolves the provider and context for writes that need repository identity
// (comments and checks), applying the fork-execution gate to detected providers.
// The returned provider is nil when nothing can render the write.
func (r *reporter) gated(f Feature, enabled func(*schema.AtmosConfiguration) bool) (gatedTarget, error) {
	p, rc := r.target(f, enabled)
	if p == nil {
		return gatedTarget{receipt: rc}, nil
	}
	ciCtx, err := r.requestContext(rc)
	if err != nil {
		return gatedTarget{receipt: rc}, err
	}
	allowFork := r.cfg != nil && r.cfg.CI.AllowUnsafeForkExecution
	if !rc.Local && ciCtx.ElevatedEvent && !allowFork {
		log.Debug("Skipping CI write on elevated event without ci.allow_unsafe_fork_execution", "feature", f)
		l := r.local()
		return gatedTarget{provider: l, receipt: r.localReceipt(l, FeatureForkGate), ciCtx: ciCtx}, nil
	}
	return gatedTarget{provider: p, receipt: rc, ciCtx: ciCtx}, nil
}

// buildCommentOptions assembles provider options for a comment request.
func buildCommentOptions(req CommentRequest, ciCtx *Context, local bool) (*PostCommentOptions, error) {
	opts := &PostCommentOptions{
		Owner:    ciCtx.RepoOwner,
		Repo:     ciCtx.RepoName,
		PRNumber: req.PR,
		Body:     req.Body,
		Behavior: CommentBehaviorCreate,
	}
	if opts.PRNumber == 0 && ciCtx.PullRequest != nil {
		opts.PRNumber = ciCtx.PullRequest.Number
	}
	if req.Key != "" {
		opts.Marker = commentMarkerPrefix + req.Key + commentMarkerSuffix
		opts.Body = opts.Marker + "\n" + req.Body
		behavior, err := provider.NormalizeBehavior(req.Behavior)
		if err != nil {
			return nil, err
		}
		opts.Behavior = behavior
	}
	if !local && opts.PRNumber <= 0 {
		return nil, errUtils.Build(errUtils.ErrCIPullRequestUnknown).
			WithHint("Pass the pull request number explicitly, for example pr=123").
			Err()
	}
	return opts, nil
}

func (r *reporter) Comment(ctx context.Context, req CommentRequest) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Comment")()

	g, err := r.gated(FeatureComments, CommentsEnabled)
	if err != nil || g.provider == nil {
		return g.receipt, err
	}
	p, rc, ciCtx := g.provider, g.receipt, g.ciCtx
	opts, err := buildCommentOptions(req, ciCtx, rc.Local)
	if err != nil {
		return rc, err
	}
	comment, err := p.PostComment(ctx, opts)
	if err != nil {
		return rc, wrapErr(errUtils.ErrCICommentPostFailed, err)
	}
	rc.Comment = comment
	return rc, nil
}

func (r *reporter) Check(ctx context.Context, req CheckRequest) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Check")()

	g, err := r.gated(FeatureChecks, ChecksEnabled)
	if err != nil || g.provider == nil {
		return g.receipt, err
	}
	p, rc, ciCtx := g.provider, g.receipt, g.ciCtx
	state := req.State
	if state == "" {
		state = CheckRunStatePending
	}
	check, err := p.CreateCheckRun(ctx, &CreateCheckRunOptions{
		Owner:      ciCtx.RepoOwner,
		Repo:       ciCtx.RepoName,
		SHA:        ciCtx.SHA,
		Name:       req.Name,
		Status:     state,
		Title:      req.Description,
		DetailsURL: checkURL(req, ciCtx),
	})
	if err != nil {
		return rc, wrapErr(errUtils.ErrCICheckRunCreateFailed, err)
	}
	rc.Check = check
	return rc, nil
}

func (r *reporter) UpdateCheck(ctx context.Context, req CheckRequest) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.UpdateCheck")()

	g, err := r.gated(FeatureChecks, ChecksEnabled)
	if err != nil || g.provider == nil {
		return g.receipt, err
	}
	p, rc, ciCtx := g.provider, g.receipt, g.ciCtx
	state := req.State
	if state == "" {
		state = CheckRunStatePending
	}
	opts := &UpdateCheckRunOptions{
		Owner:      ciCtx.RepoOwner,
		Repo:       ciCtx.RepoName,
		SHA:        ciCtx.SHA,
		Name:       req.Name,
		Status:     state,
		Title:      req.Description,
		DetailsURL: checkURL(req, ciCtx),
	}
	if conclusion, done := terminalConclusion(state); done {
		completed := time.Now().UTC()
		opts.Conclusion = conclusion
		opts.CompletedAt = &completed
	}
	check, err := p.UpdateCheckRun(ctx, opts)
	if err != nil {
		return rc, wrapErr(errUtils.ErrCICheckRunUpdateFailed, err)
	}
	rc.Check = check
	return rc, nil
}

// checkURL returns the request's details URL, defaulting to the run URL.
func checkURL(req CheckRequest, ciCtx *Context) string {
	if req.URL != "" {
		return req.URL
	}
	return ciCtx.RunURL
}
