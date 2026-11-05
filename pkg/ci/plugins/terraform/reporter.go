package terraform

import (
	"context"

	ci "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// reporterFor returns a Reporter bound to the hook's CI provider and run context. Native comments and
// commit statuses are written through it, so the fork-execution gate that holds a script's posts for a
// fork pull request on an elevated event holds the plugin's posts too. The plugin has already checked
// the feature switches before it builds a body, and the Reporter checks them again with the same
// configuration, so the two cannot disagree.
func reporterFor(ctx *plugin.HookContext) ci.Reporter {
	ciCtx := ctx.CICtx
	if ciCtx == nil {
		ciCtx = &ci.Context{}
	}
	return ci.NewReporterForProvider(ctx.Config, ctx.Provider, ciCtx)
}

// noteHeld logs when the Reporter held a write for a fork pull request, so the missing comment or
// status is explained rather than silent.
func noteHeld(rc ci.Receipt, what string, fields ...any) {
	if rc.Gate != ci.FeatureForkGate {
		return
	}
	log.Warn("Skipped "+what+" for a fork pull request on an elevated event; set ci.allow_unsafe_fork_execution to post it", fields...)
}

// postKeyedComment posts a comment identified by key through the Reporter. The key becomes the HTML
// marker "<!-- atmos:ci:<key> -->" that later runs use to find and update the comment.
func postKeyedComment(ctx *plugin.HookContext, key, summary string, behavior provider.CommentBehavior) (*provider.Comment, error) {
	prNumber := 0
	if ctx.CICtx != nil && ctx.CICtx.PullRequest != nil {
		prNumber = ctx.CICtx.PullRequest.Number
	}
	rc, err := reporterFor(ctx).Comment(context.Background(), ci.CommentRequest{
		Body:     summary,
		Key:      key,
		Behavior: behavior,
		PR:       prNumber,
		Target:   ci.CommentTargetPR,
	})
	if err != nil {
		return nil, err
	}
	noteHeld(rc, "the PR comment", "key", key)
	return rc.Comment, nil
}
