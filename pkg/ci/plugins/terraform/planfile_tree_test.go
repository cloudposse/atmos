package terraform

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
)

type sourceProvider struct {
	*resolvingProvider
	treeErr    error
	tree       string
	queue      *provider.PullRequestRevision
	queueCalls int
	treeCalls  int
}

func (p *sourceProvider) CommitTree(_ context.Context, _ *provider.Context) (string, error) {
	p.treeCalls++
	return p.tree, p.treeErr
}

func (p *sourceProvider) ReviewPullRequest(_ context.Context, _ *provider.Context) (*provider.PullRequestRevision, error) {
	p.queueCalls++
	return p.queue, nil
}

func TestPlanfileSourceTreeSurvivesCommitRewrite(t *testing.T) {
	for _, event := range []string{"pull_request", "merge_group"} {
		t.Run(event, func(t *testing.T) {
			store := newLocalPlanfileStore(t)
			info := mergedDeployInfo(t)
			prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
			ctx := mergedDeployContext(info, prov, store)
			ctx.CICtx.EventName = event
			ctx.CICtx.SHA = prHeadSHA
			require.NoError(t, os.WriteFile(info.PlanFile, []byte("reviewed-source-plan"), 0o600))
			p := &Plugin{}
			require.NoError(t, p.uploadPlanfile(ctx))
			// The landed SHA differs, but its source tree is identical to the planned
			// synthetic PR/queue commit. This exercises upload and both downloads.
			ctx.CICtx.SHA = mergeCommitSHA
			ctx.CICtx.EventName = "push"
			require.NoError(t, p.downloadPlanfileForVerification(ctx))
			assert.Equal(t, "reviewed-source-plan", readStoredPlan(t, info))
			assert.Empty(t, prov.calls, "tree provenance should avoid a merged-PR lookup")
			assert.Equal(t, mergeCommitSHA, ctx.CICtx.SHA)

			// A reference cannot resurrect an explicitly deleted/expired original.
			sourceCtx := *ctx.CICtx
			sourceCtx.SHA = prHeadSHA
			key, err := p.getArtifactKey(info, &sourceCtx)
			require.NoError(t, err)
			require.NoError(t, store.Delete(context.Background(), key))
			err = p.downloadPlanfileForVerification(ctx)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		})
	}
}

func TestQueueDeployUsesBoundPRHeadPlan(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), queue: &provider.PullRequestRevision{Number: 290, HeadSHA: prHeadSHA}}
	ctx := mergedDeployContext(info, prov, store)
	ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
	require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(ctx))
	assert.Equal(t, "reviewed-pr-plan", readStoredPlan(t, info))
	assert.Equal(t, 1, prov.queueCalls)
	assert.Empty(t, prov.calls, "queue lookup must not use the post-merge resolver")
	assert.Equal(t, mergeCommitSHA, ctx.CICtx.SHA)
}

func TestSourceTreeLookupPreservesExactCommitPriority(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	storePlan(t, store, info, mergeCommitSHA, "exact-plan")
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "same-tree"}
	require.NoError(t, (&Plugin{}).downloadPlanfileForVerification(mergedDeployContext(info, prov, store)))
	assert.Equal(t, "exact-plan", readStoredPlan(t, info))
	assert.Zero(t, prov.treeCalls)
	assert.Zero(t, prov.queueCalls)
	assert.Empty(t, prov.calls)
}

func TestSourceTreeReferenceIsRepositoryScoped(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "same-tree"}
	ctx := mergedDeployContext(info, prov, store)
	first := (&Plugin{}).planfileTreeKey(ctx)
	require.NotEmpty(t, first)
	ctx.CICtx.RepoName = "another-repo"
	assert.NotEqual(t, first, (&Plugin{}).planfileTreeKey(ctx))
	ctx.CICtx.RepoName = "repo"
	ctx.CICtx.ServerURL = "https://enterprise.example.com"
	assert.NotEqual(t, first, (&Plugin{}).planfileTreeKey(ctx))
}

func TestMissingPlanClearsPreviousRun(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	ctx := mergedDeployContext(info, newResolvingProvider(), store)
	storePlan(t, store, info, mergeCommitSHA, "previous-run")
	p := &Plugin{}
	require.NoError(t, p.downloadPlanfileForVerification(ctx))
	previousPath := info.StoredPlanFile
	require.FileExists(t, previousPath)
	key, err := p.getArtifactKey(info, ctx.CICtx)
	require.NoError(t, err)
	require.NoError(t, store.Delete(context.Background(), key))
	require.ErrorIs(t, p.downloadPlanfileForVerification(ctx), errUtils.ErrArtifactNotFound)
	assert.NoFileExists(t, previousPath)
	assert.Empty(t, info.StoredPlanFile)
}

func TestDifferentSourceTreeDoesNotReuseQueuePlan(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "old-tree"}
	prov.merged = nil
	ctx := mergedDeployContext(info, prov, store)
	ctx.CICtx.SHA = prHeadSHA
	require.NoError(t, os.WriteFile(info.PlanFile, []byte("old-queue-plan"), 0o600))
	p := &Plugin{}
	require.NoError(t, p.uploadPlanfile(ctx))
	ctx.CICtx.SHA = mergeCommitSHA
	ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
	prov.tree = "rebuilt-tree"
	require.ErrorIs(t, p.downloadPlanfileForVerification(ctx), errUtils.ErrArtifactNotFound)
	assert.Empty(t, info.StoredPlanFile)
	assert.Equal(t, 1, prov.queueCalls)
}

type failingSourceUploadStore struct{ planfile.Store }

func (s *failingSourceUploadStore) Upload(ctx context.Context, key string, files []planfile.FileEntry, metadata *planfile.Metadata) error {
	if planfile.IsSourceReference(key) {
		return os.ErrPermission
	}
	return s.Store.Upload(ctx, key, files, metadata)
}

func TestSourceReferenceFailurePreservesOriginalUpload(t *testing.T) {
	store := &failingSourceUploadStore{Store: newLocalPlanfileStore(t)}
	info := mergedDeployInfo(t)
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
	ctx := mergedDeployContext(info, prov, store)
	require.NoError(t, os.WriteFile(info.PlanFile, []byte("original-plan"), 0o600))
	p := &Plugin{}
	require.NoError(t, p.uploadPlanfile(ctx))
	require.NoError(t, p.downloadPlanfileForVerification(ctx))
	assert.Equal(t, "original-plan", readStoredPlan(t, info))
}

func TestCorruptSourceReferenceDoesNotSelectAnotherPlan(t *testing.T) {
	store := newLocalPlanfileStore(t)
	info := mergedDeployInfo(t)
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
	ctx := mergedDeployContext(info, prov, store)
	p := &Plugin{}
	storePlan(t, store, info, prHeadSHA, "otherwise-available-plan")
	files := []planfile.FileEntry{{Name: planSourceFilename, Data: strings.NewReader(`{"sha":"../../another-target"}`), Size: -1}}
	require.NoError(t, store.Upload(context.Background(), p.planfileTreeKey(ctx), files, p.buildPlanfileMetadata(ctx)))
	require.ErrorIs(t, p.downloadPlanfileForVerification(ctx), errUtils.ErrPlanfileMetadataInvalid)
	assert.Empty(t, prov.calls)
	assert.Empty(t, info.StoredPlanFile)
}
