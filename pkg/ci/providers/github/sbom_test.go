package github

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

type recordingSBOMArtifactStore struct {
	name     string
	content  []byte
	metadata *artifact.Metadata
}

func (s *recordingSBOMArtifactStore) Name() string { return "github/artifacts" }

func (s *recordingSBOMArtifactStore) Upload(_ context.Context, name string, data io.Reader, _ int64, metadata *artifact.Metadata) error {
	content, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	s.name = name
	s.content = content
	s.metadata = metadata
	return nil
}

func (*recordingSBOMArtifactStore) Download(context.Context, string) (io.ReadCloser, *artifact.Metadata, error) {
	return nil, nil, nil
}
func (*recordingSBOMArtifactStore) Delete(context.Context, string) error { return nil }
func (*recordingSBOMArtifactStore) List(context.Context, artifact.Query) ([]artifact.ArtifactInfo, error) {
	return nil, nil
}

func (*recordingSBOMArtifactStore) Exists(context.Context, string) (bool, error) { return false, nil }

func (*recordingSBOMArtifactStore) GetMetadata(context.Context, string) (*artifact.Metadata, error) {
	return nil, nil
}

func TestUploadSBOMPublishesGitHubActionsArtifact(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "cloudposse/infra-live")
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_REF_NAME", "main")
	recorded := &recordingSBOMArtifactStore{}
	previous := newSBOMArtifactStore
	newSBOMArtifactStore = func(artifact.StoreOptions) (artifact.Backend, error) { return recorded, nil }
	t.Cleanup(func() { newSBOMArtifactStore = previous })

	upload, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Filename: "atmos-sbom.spdx.json", Format: "spdx-json", Content: []byte(`{"spdxVersion":"SPDX-2.3"}`)})
	require.NoError(t, err)
	require.Equal(t, ProviderName, upload.Provider)
	require.Equal(t, "github-actions-artifact:atmos-sbom.spdx.json", upload.Location)
	require.Equal(t, "atmos-sbom.spdx.json", recorded.name)
	require.JSONEq(t, `{"spdxVersion":"SPDX-2.3"}`, string(recorded.content))
	require.Equal(t, "sbom", recorded.metadata.Component)
	require.Equal(t, "spdx-json", recorded.metadata.Custom["format"])
	require.Equal(t, "cloudposse/infra-live", recorded.metadata.Repository)
}

func TestUploadSBOMRejectsEmptyContent(t *testing.T) {
	_, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Filename: "sbom.json"})
	require.ErrorContains(t, err, "SBOM content is empty")
}
