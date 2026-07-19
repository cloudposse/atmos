package github

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

type recordingSBOMArtifactStore struct {
	name      string
	content   []byte
	metadata  *artifact.Metadata
	uploadErr error
}

func (s *recordingSBOMArtifactStore) Name() string { return "github/artifacts" }

func (s *recordingSBOMArtifactStore) Upload(_ context.Context, name string, data io.Reader, _ int64, metadata *artifact.Metadata) error {
	if s.uploadErr != nil {
		return s.uploadErr
	}
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

func TestUploadSBOMRejectsEmptyFilename(t *testing.T) {
	_, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Content: []byte("{}")})
	require.ErrorContains(t, err, "SBOM filename is required")
}

// TestUploadSBOMRequiresRepositoryContext exercises the guard that refuses to
// publish an SBOM when the GitHub Actions context can't identify a repository
// or commit to attach the artifact to (e.g. running outside a real workflow).
func TestUploadSBOMRequiresRepositoryContext(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "")

	_, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Filename: "atmos-sbom.spdx.json", Format: "spdx-json", Content: []byte(`{}`)})
	require.ErrorContains(t, err, "missing repository owner/name or commit SHA")
}

func TestUploadSBOMPropagatesArtifactStoreCreationError(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "cloudposse/infra-live")
	t.Setenv("GITHUB_SHA", "0123456789abcdef0123456789abcdef01234567")

	previous := newSBOMArtifactStore
	storeErr := errors.New("boom: no artifact backend configured")
	newSBOMArtifactStore = func(artifact.StoreOptions) (artifact.Backend, error) { return nil, storeErr }
	t.Cleanup(func() { newSBOMArtifactStore = previous })

	_, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Filename: "atmos-sbom.spdx.json", Format: "spdx-json", Content: []byte(`{}`)})
	require.ErrorIs(t, err, storeErr)
	require.ErrorContains(t, err, "create GitHub SBOM artifact store")
}

func TestUploadSBOMPropagatesArtifactUploadError(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "cloudposse/infra-live")
	t.Setenv("GITHUB_SHA", "0123456789abcdef0123456789abcdef01234567")

	uploadErr := errors.New("boom: artifact service unavailable")
	recorded := &recordingSBOMArtifactStore{uploadErr: uploadErr}
	previous := newSBOMArtifactStore
	newSBOMArtifactStore = func(artifact.StoreOptions) (artifact.Backend, error) { return recorded, nil }
	t.Cleanup(func() { newSBOMArtifactStore = previous })

	_, err := NewProvider().UploadSBOM(context.Background(), provider.SBOMReport{Filename: "atmos-sbom.spdx.json", Format: "spdx-json", Content: []byte(`{}`)})
	require.ErrorIs(t, err, uploadErr)
	require.ErrorContains(t, err, "upload SBOM as GitHub Actions artifact")
}
