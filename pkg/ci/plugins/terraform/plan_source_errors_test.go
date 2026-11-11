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
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
)

func TestPullRequestPlanfileKeyRejectsIncompleteContext(t *testing.T) {
	for _, missing := range []string{"ci context", "checkout sha", "resolved head sha"} {
		t.Run(missing, func(t *testing.T) {
			prov := newResolvingProvider()
			ctx := mergedDeployContext(mergedDeployInfo(t), prov, newLocalPlanfileStore(t))
			switch missing {
			case "ci context":
				ctx.CICtx = nil
			case "checkout sha":
				ctx.CICtx.SHA = ""
			case "resolved head sha":
				prov.merged.HeadSHA = ""
			}
			key, revision := (&Plugin{}).pullRequestPlanfileKey(ctx)
			assert.Empty(t, key)
			assert.Nil(t, revision)
			if missing == "resolved head sha" {
				assert.Equal(t, []string{mergeCommitSHA}, prov.calls)
			} else {
				assert.Empty(t, prov.calls)
			}
		})
	}
}

func TestSourceTreeUnavailableDoesNotPublishReference(t *testing.T) {
	for _, missing := range []string{"ci context", "sha", "owner", "repository", "tree", "tree lookup", "stack", "component"} {
		t.Run(missing, func(t *testing.T) {
			store := newLocalPlanfileStore(t)
			prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
			ctx := mergedDeployContext(mergedDeployInfo(t), prov, store)
			switch missing {
			case "ci context":
				ctx.CICtx = nil
			case "sha":
				ctx.CICtx.SHA = ""
			case "owner":
				ctx.CICtx.RepoOwner = ""
			case "repository":
				ctx.CICtx.RepoName = ""
			case "tree":
				prov.tree = ""
			case "tree lookup":
				prov.treeErr = os.ErrPermission
			case "stack":
				ctx.Info.Stack = ""
			case "component":
				ctx.Info.ComponentFromArg = ""
			}
			p := &Plugin{}
			assert.Empty(t, p.planfileTreeKey(ctx))
			p.uploadPlanfileTreeAlias(ctx, store, nil)
			files, err := store.List(context.Background(), planfile.Query{All: true})
			require.NoError(t, err)
			assert.Empty(t, files, "incomplete source evidence must never publish a reference")
		})
	}
}

func TestTreeReferenceRejectsMalformedBundles(t *testing.T) {
	for _, tc := range []struct{ name, filename, body string }{
		{"wrong file", planfile.PlanFilename, `{"sha":"` + prHeadSHA + `"}`},
		{"invalid json", planSourceFilename, `{`},
		{"short hex sha", planSourceFilename, `{"sha":"abcd"}`},
		{"nonhex sha", planSourceFilename, `{"sha":"gggggggggggggggggggggggggggggggggggggggg"}`},
		{"oversized reference", planSourceFilename, strings.Repeat(" ", 1024) + `{"sha":"` + prHeadSHA + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
			prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
			ctx := mergedDeployContext(mergedDeployInfo(t), prov, store)
			p := &Plugin{}
			key := p.planfileTreeKey(ctx)
			files := []planfile.FileEntry{{Name: tc.filename, Data: strings.NewReader(tc.body), Size: int64(len(tc.body))}}
			require.NoError(t, store.Upload(context.Background(), key, files, p.buildPlanfileMetadata(ctx)))
			results, gotKey, err := p.downloadTreePlanfile(ctx, store, key)
			require.ErrorIs(t, err, errUtils.ErrPlanfileMetadataInvalid)
			assert.Nil(t, results)
			assert.Equal(t, key, gotKey)
			assert.Equal(t, []string{key}, store.calls, "invalid references must not initiate a destination download")
		})
	}
}

func TestTreeReferenceRejectsMissingTargetIdentity(t *testing.T) {
	store := &recordingPlanfileStore{Store: newLocalPlanfileStore(t)}
	prov := &sourceProvider{resolvingProvider: newResolvingProvider(), tree: "source-tree"}
	ctx := mergedDeployContext(mergedDeployInfo(t), prov, store)
	p := &Plugin{}
	key := p.planfileTreeKey(ctx)
	p.uploadPlanfileTreeAlias(ctx, store, p.buildPlanfileMetadata(ctx))
	ctx.Info.ComponentFromArg = ""
	results, gotKey, err := p.downloadTreePlanfile(ctx, store, key)
	require.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
	assert.Nil(t, results)
	assert.Equal(t, key, gotKey)
	assert.Equal(t, []string{key}, store.calls)
}

func TestDownloadStopsWhenPreviousPlanCannotBeCleared(t *testing.T) {
	info := mergedDeployInfo(t)
	stalePath := filepath.Join(filepath.Dir(info.PlanFile), planfile.StoredPlanPrefix+planfile.PlanFilename)
	require.NoError(t, os.Mkdir(stalePath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stalePath, "keep"), []byte("do not delete"), 0o600))
	info.StoredPlanFile = stalePath
	prov := newResolvingProvider()
	ctx := mergedDeployContext(info, prov, nil)
	ctx.CreatePlanfileStore = func() (any, error) { t.Fatal("cleanup failure must stop before accessing storage"); return nil, nil }
	err := (&Plugin{}).downloadPlanfileForVerification(ctx)
	require.ErrorIs(t, err, errUtils.ErrPlanfileDownloadFailed)
	assert.Empty(t, info.StoredPlanFile)
	assert.Empty(t, prov.calls)
	assert.FileExists(t, filepath.Join(stalePath, "keep"))
}
