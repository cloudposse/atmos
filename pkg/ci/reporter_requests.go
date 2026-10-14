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
// Under an elevated event (pull_request_target, workflow_run) the write is held only when the
// pull request comes from a fork, unless ci.allow_unsafe_fork_execution is set. A same-repository
// pull request, or a run with no pull request, posts normally. Env, path, and SARIF writes go
// through routeTo and are held by the same rule.
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
	if !rc.Local && r.forkHeld(ciCtx) {
		log.Debug("Skipping CI write for a fork pull request on an elevated event without ci.allow_unsafe_fork_execution", "feature", f)
		l := r.local()
		return gatedTarget{provider: l, receipt: r.localReceipt(l, FeatureForkGate), ciCtx: ciCtx}, nil
	}
	return gatedTarget{provider: p, receipt: rc, ciCtx: ciCtx}, nil
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
	detailsURL := checkURL(req, ciCtx)
	check, err := p.CreateCheckRun(ctx, &CreateCheckRunOptions{
		Owner:      ciCtx.RepoOwner,
		Repo:       ciCtx.RepoName,
		SHA:        ciCtx.SHA,
		Name:       req.Name,
		Status:     state,
		Title:      req.Description,
		DetailsURL: detailsURL,
	})
	if err != nil {
		return rc, wrapErr(errUtils.ErrCICheckRunCreateFailed, err)
	}
	rc.Check = withDetailsURL(check, detailsURL)
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
		ID:         req.ID,
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
	rc.Check = withDetailsURL(check, opts.DetailsURL)
	return rc, nil
}

// checkURL returns the request's details URL, defaulting to the run URL.
func checkURL(req CheckRequest, ciCtx *Context) string {
	if req.URL != "" {
		return req.URL
	}
	return ciCtx.RunURL
}

// withDetailsURL fills in the details URL a provider did not report, so the receipt always names
// where the check links to.
func withDetailsURL(check *CheckRun, url string) *CheckRun {
	if check != nil && check.DetailsURL == "" {
		check.DetailsURL = url
	}
	return check
}
