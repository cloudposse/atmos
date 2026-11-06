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
//  2. A reference to a plan for the identical source tree, if one was published.
//  3. The planfile stored under the head commit of the pull request merged as the current commit.
//     Queue runs resolve the head bound to the active queue entry instead.
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

	// Equivalent source trees cover PR synthetic commits and queue commits that
	// land with a different SHA. Never try another source after a storage error.
	if treeKey := p.planfileTreeKey(ctx); treeKey != "" {
		treeResults, sourceKey, treeErr := p.downloadTreePlanfile(ctx, store, treeKey)
		if treeErr == nil {
			log.Info("Using the plan stored for the same source tree", "key", sourceKey, "sha", ctx.CICtx.SHA)
			return treeResults, sourceKey, nil
		}
		if !errors.Is(treeErr, errUtils.ErrArtifactNotFound) {
			return nil, treeKey, treeErr
		}
	}

	headKey, merged := p.pullRequestPlanfileKey(ctx)
	if merged == nil || headKey == key {
		return nil, key, err
	}

	results, _, headErr := store.Download(context.Background(), headKey)
	if headErr != nil {
		return nil, headKey, errors.Join(err, headErr)
	}
	log.Info("Using the plan stored for the pull request's head commit",
		"pr", merged.Number, "head_sha", merged.HeadSHA, "merge_sha", ctx.CICtx.SHA)
	return results, headKey, nil
}

// pullRequestPlanfileKey returns the planfile key for the head commit of the pull request
// associated with the current checkout, and that pull request. It returns a nil pull request when there
// is none or it cannot be determined.
func (p *Plugin) pullRequestPlanfileKey(ctx *plugin.HookContext) (string, *provider.PullRequestRevision) {
	defer perf.Track(ctx.Config, "terraform.Plugin.pullRequestPlanfileKey")()

	// Review runs resolve only a revision bound to their event or active queue
	// entry. Post-merge runs resolve the PR that actually landed as this SHA.
	if ctx.CICtx == nil || ctx.CICtx.SHA == "" {
		return "", nil
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), planSourceLookupTimeout)
	defer cancel()
	var merged *provider.PullRequestRevision
	var err error
	if ctx.CICtx.MergeQueue != nil || ctx.CICtx.PullRequest != nil {
		resolver, ok := ctx.Provider.(provider.ReviewPullRequestResolver)
		if !ok {
			return "", nil
		}
		merged, err = resolver.ReviewPullRequest(lookupCtx, ctx.CICtx)
	} else {
		resolver, ok := ctx.Provider.(provider.MergedPullRequestResolver)
		if !ok {
			return "", nil
		}
		merged, err = resolver.MergedPullRequest(lookupCtx, ctx.CICtx, ctx.CICtx.SHA)
	}
	if err != nil {
		// The most common cause is a workflow token without the `pull-requests: read` permission.
		log.Warn("Cannot resolve the pull request plan source; grant the workflow `pull-requests: read` to use its plan",
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
