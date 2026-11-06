package terraform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const planSourceLookupTimeout = 30 * time.Second

// planfileTreeKey is a secondary key; existing commit keys remain authoritative.
// Repository and server scope prevent shared stores from crossing repositories.
func (p *Plugin) planfileTreeKey(ctx *plugin.HookContext) string {
	defer perf.Track(ctx.Config, "terraform.Plugin.planfileTreeKey")()
	resolver, ok := ctx.Provider.(provider.CommitTreeResolver)
	if !ok || ctx.CICtx == nil || ctx.CICtx.SHA == "" || ctx.CICtx.RepoOwner == "" || ctx.CICtx.RepoName == "" {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), planSourceLookupTimeout)
	defer cancel()
	tree, err := resolver.CommitTree(lookupCtx, ctx.CICtx)
	if err != nil {
		log.Warn("Cannot resolve the source tree for planfile correlation", "error", err)
		return ""
	}
	if tree == "" {
		return ""
	}
	scope := sha256.Sum256([]byte(ctx.CICtx.ServerURL + "/" + ctx.CICtx.RepoOwner + "/" + ctx.CICtx.RepoName))
	treeCtx := *ctx.CICtx
	treeCtx.SHA = fmt.Sprintf("tree-%x-%s", scope, tree)
	key, err := p.getArtifactKey(ctx.Info, &treeCtx)
	if err != nil {
		return ""
	}
	return key
}

// uploadPlanfileTreeAlias stores a small reference to the original commit-keyed
// artifact. Deleting or expiring that artifact also invalidates the reference.
func (p *Plugin) uploadPlanfileTreeAlias(ctx *plugin.HookContext, store planfile.Store, metadata *planfile.Metadata) {
	defer perf.Track(ctx.Config, "terraform.Plugin.uploadPlanfileTreeAlias")()
	key := p.planfileTreeKey(ctx)
	if key == "" {
		return
	}
	data, err := json.Marshal(planSourceReference{SHA: ctx.CICtx.SHA})
	if err != nil {
		return
	}
	files := []planfile.FileEntry{{Name: planSourceFilename, Data: bytes.NewReader(data), Size: int64(len(data))}}
	if err := store.Upload(context.Background(), key, files, metadata); err != nil {
		log.Warn("Cannot store the planfile source-tree reference", "key", key, "error", err)
	}
}

const (
	planSourceFilename  = "plan-source.json"
	planSourceSHALength = 40
)

type planSourceReference struct {
	SHA string `json:"sha"`
}

func (p *Plugin) downloadTreePlanfile(ctx *plugin.HookContext, store planfile.Store, treeKey string) ([]planfile.FileResult, string, error) {
	defer perf.Track(ctx.Config, "terraform.Plugin.downloadTreePlanfile")()
	files, _, err := store.Download(context.Background(), treeKey)
	if err != nil {
		return nil, treeKey, err
	}
	defer func() {
		for _, file := range files {
			file.Data.Close()
		}
	}()
	if len(files) != 1 || files[0].Name != planSourceFilename {
		return nil, treeKey, errUtils.ErrPlanfileMetadataInvalid
	}
	var source planSourceReference
	if err := json.NewDecoder(io.LimitReader(files[0].Data, 1024)).Decode(&source); err != nil {
		return nil, treeKey, fmt.Errorf("%w: %w", errUtils.ErrPlanfileMetadataInvalid, err)
	}
	// References point only to commit keys in the same stack/component namespace,
	// never to another reference or an arbitrary storage path.
	if _, err := hex.DecodeString(source.SHA); err != nil || len(source.SHA) != planSourceSHALength {
		return nil, treeKey, errUtils.ErrPlanfileMetadataInvalid
	}
	sourceCtx := *ctx.CICtx
	sourceCtx.SHA = source.SHA
	key, err := p.getArtifactKey(ctx.Info, &sourceCtx)
	if err != nil {
		return nil, treeKey, err
	}
	results, _, err := store.Download(context.Background(), key)
	return results, key, err
}
