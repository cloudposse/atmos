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

// downloadStoredPlanfile selects the stored plan to verify a deploy against:
//
//  1. The plan for the checked-out commit.
//  2. A reference to a plan for the identical source tree.
//  3. The PR-head plan associated with the checkout: resolved from the PR event,
//     an active queue entry, or the PR merged as the checked-out commit.
//
// Only a missing artifact permits trying another source. Deploy compares the
// selected plan with a fresh plan of its checkout using the configured policy.
// This function returns the downloaded files and their source key.
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

	headKey, revision := p.pullRequestPlanfileKey(ctx)
	if revision == nil || headKey == key {
		return nil, key, err
	}

	results, _, headErr := store.Download(context.Background(), headKey)
	if headErr != nil {
		return nil, headKey, errors.Join(err, headErr)
	}
	log.Info("Using the plan stored for the pull request's head commit",
		"pr", revision.Number, "head_sha", revision.HeadSHA, "checkout_sha", ctx.CICtx.SHA)
	return results, headKey, nil
}

// pullRequestPlanfileKey resolves the PR revision associated with the checkout
// and its planfile key. An unavailable or invalid revision returns an empty key.
func (p *Plugin) pullRequestPlanfileKey(ctx *plugin.HookContext) (string, *provider.PullRequestRevision) {
	defer perf.Track(ctx.Config, "terraform.Plugin.pullRequestPlanfileKey")()

	// Review runs resolve only a revision bound to their event or active queue
	// entry. Post-merge runs resolve the PR that actually landed as this SHA.
	if ctx.CICtx == nil || ctx.CICtx.SHA == "" {
		return "", nil
	}
	revision, err := resolvePullRequestRevision(ctx)
	if err != nil {
		// The most common cause is a workflow token without the `pull-requests: read` permission.
		log.Warn("Cannot resolve the pull request plan source; grant the workflow `pull-requests: read` to use its plan",
			"sha", ctx.CICtx.SHA, "error", err)
		return "", nil
	}
	if revision == nil {
		return "", nil
	}

	headCtx := *ctx.CICtx
	headCtx.SHA = revision.HeadSHA
	key, err := p.getArtifactKey(ctx.Info, &headCtx)
	if err != nil {
		log.Debug("Cannot build the planfile key for the pull request's head commit", "pr", revision.Number, "error", err)
		return "", nil
	}
	return key, revision
}

// resolvePullRequestRevision asks the provider for the PR revision associated
// with the checkout. It returns nil when the provider cannot resolve one.
func resolvePullRequestRevision(ctx *plugin.HookContext) (*provider.PullRequestRevision, error) {
	lookupCtx, cancel := context.WithTimeout(context.Background(), planSourceLookupTimeout)
	defer cancel()
	// Only PR review events bind a revision to their payload. Other events that
	// carry PR info, such as workflow_run, resolve the PR merged as this SHA.
	if ctx.CICtx.MergeQueue != nil || (ctx.CICtx.PullRequest != nil && isPullRequestReviewEvent(ctx.CICtx.EventName)) {
		resolver, ok := ctx.Provider.(provider.ReviewPullRequestResolver)
		if !ok {
			return nil, nil
		}
		return resolver.ReviewPullRequest(lookupCtx, ctx.CICtx)
	}
	resolver, ok := ctx.Provider.(provider.MergedPullRequestResolver)
	if !ok {
		return nil, nil
	}
	return resolver.MergedPullRequest(lookupCtx, ctx.CICtx, ctx.CICtx.SHA)
}

// isPullRequestReviewEvent reports whether the event reviews a pull request
// revision whose head is recorded in the event payload.
func isPullRequestReviewEvent(eventName string) bool {
	return eventName == "pull_request" || eventName == "pull_request_target"
}
