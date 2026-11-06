package terraform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/artifact/local"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/adapter"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	mergeCommitSHA = "fbdaf104ec6e2ffeb329f5badb51fb220ec7dcea"
	prHeadSHA      = "b1fb6745d01e6a3f4a7738e0f73de40f37f92617"
)

// resolvingProvider is a CI provider that can map a landed commit to its merged pull request.
type resolvingProvider struct {
	*mockProvider
	merged *provider.PullRequestRevision
	err    error
	calls  []string
}

func (r *resolvingProvider) MergedPullRequest(_ context.Context, _ *provider.Context, sha string) (*provider.PullRequestRevision, error) {
	r.calls = append(r.calls, sha)
	return r.merged, r.err
}

func newResolvingProvider() *resolvingProvider {
	return &resolvingProvider{
		mockProvider: newMockProvider(),
		merged:       &provider.PullRequestRevision{Number: 290, HeadSHA: prHeadSHA},
	}
}

func newLocalPlanfileStore(t *testing.T) planfile.Store {
	t.Helper()
	backend, err := local.NewStore(artifact.StoreOptions{Options: map[string]any{"path": t.TempDir()}})
	require.NoError(t, err)
	return adapter.NewStore(artifact.NewBundledStore(backend))
}

func mergedDeployInfo(t *testing.T) *schema.ConfigAndStacksInfo {
	t.Helper()
	return &schema.ConfigAndStacksInfo{
		Stack:            "core-usw1-services",
		ComponentFromArg: "growbox-backup-writer",
		PlanFile:         filepath.Join(t.TempDir(), "plan.tfplan"),
	}
}

// storePlan uploads a planfile whose content is body under the key for sha.
func storePlan(t *testing.T, store planfile.Store, info *schema.ConfigAndStacksInfo, sha, body string) {
	t.Helper()
	key, err := (&Plugin{}).getArtifactKey(info, &provider.Context{SHA: sha})
	require.NoError(t, err)
	meta := &planfile.Metadata{Metadata: artifact.Metadata{Stack: info.Stack, Component: info.ComponentFromArg, SHA: sha}}
	files := []planfile.FileEntry{{Name: planfile.PlanFilename, Data: strings.NewReader(body), Size: int64(len(body))}}
	require.NoError(t, store.Upload(context.Background(), key, files, meta))
}

func mergedDeployContext(info *schema.ConfigAndStacksInfo, prov provider.Provider, store planfile.Store) *plugin.HookContext {
	return &plugin.HookContext{
		Config:              &schema.AtmosConfiguration{},
		Command:             "deploy",
		Provider:            prov,
		CICtx:               &provider.Context{SHA: mergeCommitSHA, Branch: "main", RepoOwner: "owner", RepoName: "repo"},
		Info:                info,
		CreatePlanfileStore: func() (any, error) { return store, nil },
	}
}

func readStoredPlan(t *testing.T, info *schema.ConfigAndStacksInfo) string {
	t.Helper()
	require.NotEmpty(t, info.StoredPlanFile, "a stored planfile should have been written for verification")
	data, err := os.ReadFile(info.StoredPlanFile)
	require.NoError(t, err)
	return string(data)
}

func TestDownloadPlanfileForVerification_FallsBackToMergedPRHead(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
	prov := newResolvingProvider()

	require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(mergedDeployContext(info, prov, store)))

	assert.Equal(t, "reviewed-pr-plan", readStoredPlan(t, info))
	assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
}

func TestDownloadPlanfileForVerification_PrefersPlanStoredForMergeCommit(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	storePlan(t, store, info, mergeCommitSHA, "post-merge-plan")
	storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
	prov := newResolvingProvider()

	require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(mergedDeployContext(info, prov, store)))

	assert.Equal(t, "post-merge-plan", readStoredPlan(t, info))
	assert.Empty(t, prov.calls, "the pull request lookup must not run when the merge commit has its own plan")
}

func TestDownloadPlanfileForVerification_NoFallback(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(ctx *plugin.HookContext, prov *resolvingProvider)
		storeHeadPlan bool
		wantLookups   int
	}{
		{
			name:        "pull request head has no stored plan either",
			wantLookups: 1,
		},
		{
			name:          "commit is not a merged pull request's merge commit",
			setup:         func(_ *plugin.HookContext, prov *resolvingProvider) { prov.merged = nil },
			storeHeadPlan: true,
			wantLookups:   1,
		},
		{
			name:          "pull request lookup fails",
			setup:         func(_ *plugin.HookContext, prov *resolvingProvider) { prov.merged, prov.err = nil, assert.AnError },
			storeHeadPlan: true,
			wantLookups:   1,
		},
		{
			name: "pull request event plans its own commit",
			setup: func(ctx *plugin.HookContext, _ *resolvingProvider) {
				ctx.CICtx.EventName = "pull_request"
				ctx.CICtx.PullRequest = &provider.PRInfo{Number: 290}
			},
			storeHeadPlan: true,
		},
		{
			name: "merge queue event plans its own commit",
			setup: func(ctx *plugin.HookContext, _ *resolvingProvider) {
				ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
			},
			storeHeadPlan: true,
		},
		{
			name:          "provider cannot resolve merged pull requests",
			setup:         func(ctx *plugin.HookContext, _ *resolvingProvider) { ctx.Provider = newMockProvider() },
			storeHeadPlan: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newLocalPlanfileStore(t)
			info := mergedDeployInfo(t)
			if tt.storeHeadPlan {
				storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
			}
			prov := newResolvingProvider()
			ctx := mergedDeployContext(info, prov, store)
			if tt.setup != nil {
				tt.setup(ctx, prov)
			}

			err := (&Plugin{}).downloadPlanfileForVerification(ctx)

			require.ErrorIs(t, err, errUtils.ErrPlanfileDownloadFailed)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assert.Empty(t, info.StoredPlanFile)
			assert.Len(t, prov.calls, tt.wantLookups)
		})
	}
}

// A workflow_run carries the triggering PR's info but no review payload, so it
// must resolve the PR merged as the checkout rather than a review revision.
func TestDownloadPlanfileForVerification_WorkflowRunUsesMergedPR(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
	prov := newResolvingProvider()
	ctx := mergedDeployContext(info, prov, store)
	ctx.CICtx.EventName = "workflow_run"
	ctx.CICtx.PullRequest = &provider.PRInfo{Number: 290}

	require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(ctx))

	assert.Equal(t, "reviewed-pr-plan", readStoredPlan(t, info))
	assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
}
