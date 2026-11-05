package terraform

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// downloadStoredPlanfile downloads the planfile to verify a deploy against, in this order:
//
//  1. The planfile stored under key, the key for the current commit. After a merge, this is the
//     plan of the landed commit itself, for example from a plan workflow dispatched on
//     pull_request.merged and applied once approved.
//  2. The planfile stored under the head commit of the pull request merged as the current commit.
//     A deploy dispatched straight after a squash, merge, or rebase merge runs on the landed
//     commit, while the pull request's reviewed plan was stored under its head commit.
//
// The second step runs only when nothing is stored under key. Either way, deploy verification
// compares the stored plan with a fresh plan of the landed commit, so a base branch that has
// diverged since the plan fails verification instead of applying a stale plan.
//
// It returns the downloaded files and the key they came from.
func (p *Plugin) downloadStoredPlanfile(ctx *plugin.HookContext, store planfile.Store, key string) ([]planfile.FileResult, string, error) {
	defer perf.Track(ctx.Config, "terraform.Plugin.downloadStoredPlanfile")()

	results, _, err := store.Download(context.Background(), key)
	if err == nil || !errors.Is(err, errUtils.ErrArtifactNotFound) {
		return results, key, err
	}

	headKey, merged := p.mergedPullRequestPlanfileKey(ctx)
	if merged == nil || headKey == key {
		return nil, key, err
	}

	results, _, headErr := store.Download(context.Background(), headKey)
	if headErr != nil {
		return nil, headKey, errors.Join(err, headErr)
	}
	log.Info("Using the plan stored for the merged pull request's head commit",
		"pr", merged.Number, "head_sha", merged.HeadSHA, "merge_sha", ctx.CICtx.SHA)
	return results, headKey, nil
}

// mergedPullRequestPlanfileKey returns the planfile key for the head commit of the pull request
// merged as the current commit, and that pull request. It returns a nil pull request when there
// is none or it cannot be determined.
func (p *Plugin) mergedPullRequestPlanfileKey(ctx *plugin.HookContext) (string, *provider.MergedPullRequest) {
	defer perf.Track(ctx.Config, "terraform.Plugin.mergedPullRequestPlanfileKey")()

	// Pull request and merge-queue runs plan the commit they run on, so there is nothing to map.
	if ctx.CICtx == nil || ctx.CICtx.SHA == "" || ctx.CICtx.PullRequest != nil || ctx.CICtx.MergeQueue != nil {
		return "", nil
	}
	resolver, ok := ctx.Provider.(provider.MergedPullRequestResolver)
	if !ok {
		return "", nil
	}

	merged, err := resolver.MergedPullRequest(context.Background(), ctx.CICtx, ctx.CICtx.SHA)
	if err != nil {
		// The most common cause is a workflow token without the `pull-requests: read` permission.
		log.Warn("Cannot look up the pull request merged as the current commit; grant the workflow `pull-requests: read` to use its plan",
			"sha", ctx.CICtx.SHA, "error", err)
		return "", nil
	}
	if merged == nil {
		return "", nil
	}

	headCtx := *ctx.CICtx
	headCtx.SHA = merged.HeadSHA
	key, err := p.getArtifactKey(ctx.Info, &headCtx)
	if err != nil {
		log.Debug("Cannot build the planfile key for the merged pull request's head commit", "pr", merged.Number, "error", err)
		return "", nil
	}
	return key, merged
}
