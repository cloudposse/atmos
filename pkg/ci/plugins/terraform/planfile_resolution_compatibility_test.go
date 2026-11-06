package terraform

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recordingPlanfileStore keeps real local-store behavior while allowing a specific
// download to fail. Calls expose accidental fallback or repeated downloads.
type recordingPlanfileStore struct {
	planfile.Store
	failures map[string]error
	calls    []string
}

func (s *recordingPlanfileStore) Download(ctx context.Context, key string) ([]planfile.FileResult, *planfile.Metadata, error) {
	s.calls = append(s.calls, key)
	if err := s.failures[key]; err != nil {
		return nil, nil, err
	}
	return s.Store.Download(ctx, key)
}

func TestDownloadPlanfileForVerification_ExactCommitAcrossEvents(t *testing.T) {
	for _, event := range []string{"push", "workflow_dispatch", "pull_request", "merge_group"} {
		t.Run(event, func(t *testing.T) {
			store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
			info := mergedDeployInfo(t)
			storePlan(t, store, info, mergeCommitSHA, "checked-out-commit-plan")
			storePlan(t, store, info, prHeadSHA, "pr-head-plan")
			prov := newResolvingProvider()
			ctx := mergedDeployContext(info, prov, store)
			ctx.CICtx.EventName = event
			if event == "pull_request" {
				ctx.CICtx.PullRequest = &provider.PRInfo{Number: 290}
			}
			if event == "merge_group" {
				ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
			}
			require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(ctx))
			assert.Equal(t, "checked-out-commit-plan", readStoredPlan(t, info))
			assert.Empty(t, prov.calls)
			assert.Len(t, store.calls, 1)
		})
	}
}

func TestOnBeforeDeploy_MergedPRFallbackIndependentOfPro(t *testing.T) {
	for _, proEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("pro=%t", proEnabled), func(t *testing.T) {
			store := newLocalPlanfileStore(t)
			info := mergedDeployInfo(t)
			storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
			prov := newResolvingProvider()
			ctx := mergedDeployContext(info, prov, store)
			ctx.Config.Settings.Pro.Enabled = proEnabled
			ctx.Config.CI.Enabled = true
			// Disable unrelated check-run creation; exercise the real deploy download gate.
			checksEnabled := false
			ctx.Config.CI.Checks.Enabled = &checksEnabled
			ctx.Config.Components.Terraform.Planfiles = schema.PlanfilesConfig{Priority: []string{"local"}}
			require.NoError(t, (&Plugin{}).onBeforeDeploy(ctx))
			assert.Equal(t, "reviewed-pr-plan", readStoredPlan(t, info))
			assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
		})
	}
}

func TestDownloadPlanfileForVerification_DownloadFailures(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{
		{"permission denied", os.ErrPermission},
		{"timeout", context.DeadlineExceeded},
		{"corrupt artifact", errUtils.ErrArtifactIntegrityFailed},
	} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fallback=%t", failure.name, fallback), func(t *testing.T) {
				store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t), failures: map[string]error{}}
				info := mergedDeployInfo(t)
				prov := newResolvingProvider()
				ctx := mergedDeployContext(info, prov, store)
				currentKey, err := (&Plugin{}).getArtifactKey(info, ctx.CICtx)
				require.NoError(t, err)
				headKey, err := (&Plugin{}).getArtifactKey(info, &provider.Context{SHA: prHeadSHA})
				require.NoError(t, err)
				storePlan(t, store, info, prHeadSHA, "available-pr-plan")
				wantCalls := []string{currentKey}
				if fallback {
					store.failures[headKey] = fmt.Errorf("backend: %w", failure.err)
					wantCalls = append(wantCalls, headKey)
				} else {
					store.failures[currentKey] = fmt.Errorf("backend: %w", failure.err)
				}
				err = (&Plugin{}).downloadPlanfileForVerification(ctx)
				require.ErrorIs(t, err, errUtils.ErrPlanfileDownloadFailed)
				require.ErrorIs(t, err, failure.err)
				assert.Equal(t, wantCalls, store.calls)
				assert.Empty(t, info.StoredPlanFile)
				if fallback {
					assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
				} else {
					assert.Empty(t, prov.calls, "a backend failure must not select another commit's plan")
				}
			})
		}
	}
}

func TestDownloadPlanfileForVerification_FallbackPreservesTarget(t *testing.T) {
	for _, dimension := range []string{"stack", "component"} {
		t.Run(dimension, func(t *testing.T) {
			store := newLocalPlanfileStore(t)
			info := mergedDeployInfo(t)
			other := *info
			if dimension == "stack" {
				other.Stack = "another-stack"
			} else {
				other.ComponentFromArg = "another-component"
			}
			storePlan(t, store, &other, prHeadSHA, "wrong-target-plan")
			prov := newResolvingProvider()
			ctx := mergedDeployContext(info, prov, store)
			err := (&Plugin{}).downloadPlanfileForVerification(ctx)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assert.Empty(t, info.StoredPlanFile)
			storePlan(t, store, info, prHeadSHA, "correct-target-plan")
			require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(ctx))
			assert.Equal(t, "correct-target-plan", readStoredPlan(t, info))
			assert.Equal(t, mergeCommitSHA, ctx.CICtx.SHA, "fallback must not change the deployment identity")
		})
	}
}

// Providers without the optional queue/tree capabilities retain exact-commit
// queue behavior. They must not guess a mutable PR head or an older queue attempt.
func TestDownloadPlanfileForVerification_QueuePlansRequireMatchingCommit(t *testing.T) {
	const oldQueueSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, queued := range []bool{true, false} {
		t.Run(fmt.Sprintf("inside_queue=%t", queued), func(t *testing.T) {
			store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
			info := mergedDeployInfo(t)
			storePlan(t, store, info, oldQueueSHA, "previous-queue-plan")
			prov := newResolvingProvider()
			ctx := mergedDeployContext(info, prov, store)
			if queued {
				ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
				storePlan(t, store, info, prHeadSHA, "mutable-pr-head-plan")
			}
			err := (&Plugin{}).downloadPlanfileForVerification(ctx)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assert.Empty(t, info.StoredPlanFile)
			if queued {
				assert.Empty(t, prov.calls)
				assert.Len(t, store.calls, 1)
			} else {
				assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
				assert.Len(t, store.calls, 2)
			}
		})
	}
}

func TestDownloadPlanfileForVerification_SameHeadDoesNotRetry(t *testing.T) {
	store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
	info := mergedDeployInfo(t)
	prov := newResolvingProvider()
	prov.merged.HeadSHA = mergeCommitSHA
	ctx := mergedDeployContext(info, prov, store)
	err := (&Plugin{}).downloadPlanfileForVerification(ctx)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Len(t, store.calls, 1, "a resolver returning the current SHA must not trigger a second download")
	assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
	assert.Empty(t, info.StoredPlanFile)
}

func TestDownloadPlanfileForVerification_UnresolvedQueueDoesNotUseMergedPRResolver(t *testing.T) {
	store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
	info := mergedDeployInfo(t)
	storePlan(t, store, info, prHeadSHA, "unrelated-pr-plan")
	prov := newResolvingProvider()
	ctx := mergedDeployContext(info, prov, store)
	ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{HeadRef: "unrecognized-queue-ref"}
	err := (&Plugin{}).downloadPlanfileForVerification(ctx)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Empty(t, prov.calls)
	assert.Len(t, store.calls, 1)
	assert.Empty(t, info.StoredPlanFile)
}
