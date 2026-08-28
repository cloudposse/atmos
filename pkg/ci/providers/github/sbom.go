package github

import (
	"bytes"
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	githubartifact "github.com/cloudposse/atmos/pkg/ci/artifact/github"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
	atmosversion "github.com/cloudposse/atmos/pkg/version"
)

var newSBOMArtifactStore = githubartifact.NewStore

// UploadSBOM publishes an Atmos-generated SBOM as a GitHub Actions artifact.
// GitHub's dependency-graph SBOM endpoints do not accept submitted SBOMs, so
// this intentionally preserves the generated document as a run artifact rather
// than claiming dependency-graph ingestion.
func (p *Provider) UploadSBOM(ctx context.Context, report provider.SBOMReport) (*provider.SBOMUpload, error) {
	defer perf.Track(nil, "github.Provider.UploadSBOM")()

	if len(report.Content) == 0 {
		return nil, fmt.Errorf("%w: SBOM content is empty", errUtils.ErrCIOperationNotSupported)
	}
	if report.Filename == "" {
		return nil, fmt.Errorf("%w: SBOM filename is required", errUtils.ErrCIOperationNotSupported)
	}
	cictx, err := p.Context()
	if err != nil {
		return nil, fmt.Errorf("get GitHub Actions context: %w", err)
	}
	if cictx.RepoOwner == "" || cictx.RepoName == "" || cictx.SHA == "" {
		return nil, fmt.Errorf("%w: missing repository owner/name or commit SHA", errUtils.ErrCIOperationNotSupported)
	}
	store, err := newSBOMArtifactStore(artifact.StoreOptions{Options: map[string]any{"owner": cictx.RepoOwner, "repo": cictx.RepoName}})
	if err != nil {
		return nil, fmt.Errorf("create GitHub SBOM artifact store: %w", err)
	}
	metadata := &artifact.Metadata{
		Stack:        "sbom",
		Component:    "sbom",
		SHA:          cictx.SHA,
		Branch:       cictx.Branch,
		RunID:        cictx.RunID,
		Repository:   cictx.Repository,
		CreatedAt:    time.Now().UTC(),
		AtmosVersion: atmosversion.Version,
		Custom:       map[string]string{"format": report.Format},
	}
	if err := store.Upload(ctx, report.Filename, bytes.NewReader(report.Content), int64(len(report.Content)), metadata); err != nil {
		return nil, fmt.Errorf("upload SBOM as GitHub Actions artifact: %w", err)
	}
	return &provider.SBOMUpload{Provider: ProviderName, Location: "github-actions-artifact:" + report.Filename}, nil
}
