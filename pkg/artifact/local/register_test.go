package local

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterWith_RegistersBothKindsAsSeparateBackends(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry))
	assert.Equal(t, []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive}, Kinds())
	for _, kind := range Kinds() {
		assert.True(t, registry.IsRegistered(kind), kind)
	}

	dirDesc, ok := registry.Lookup(artifact.RepositoryKindLocalDir)
	require.True(t, ok)
	archiveDesc, ok := registry.Lookup(artifact.RepositoryKindLocalArchive)
	require.True(t, ok)
	assert.Equal(t, []artifact.Packaging{artifact.PackagingNone}, dirDesc.Capabilities.Packagings)
	assert.Contains(t, archiveDesc.Capabilities.Packagings, artifact.PackagingZip)
	assert.Equal(t, artifact.DeleteObject, dirDesc.Capabilities.Delete, "the registered descriptor carries the backend's real capabilities")
	assert.True(t, archiveDesc.AllowsPermissions)
	assert.False(t, dirDesc.AllowsPermissions)
	assert.Equal(t, artifact.PackagingTgz, archiveDesc.DefaultPackaging)
	assert.Equal(t, []string{artifact.OptionPath}, archiveDesc.PathOptions)

	require.ErrorIs(t, RegisterWith(registry), errUtils.ErrArtifactConflict, "double registration is an error, not a silent replacement")
}

func TestRegisteredFactories_ConstructRepositories(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry))
	root := t.TempDir()

	dir, err := registry.New(context.Background(), "dir", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": filepath.Join(root, "dir")},
	})
	require.NoError(t, err)
	archive, err := registry.New(context.Background(), "archive", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalArchive, Permissions: "0640", Packaging: "zip",
		Options: map[string]any{"path": filepath.Join(root, "archive"), "key_pattern": `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.zip`},
	})
	require.NoError(t, err)
	assert.IsType(t, dirStore{}, dir.(*Repository).store)
	assert.IsType(t, archiveStore{}, archive.(*Repository).store)
	assert.Equal(t, artifact.PackagingZip, archive.(*Repository).packaging)
	assert.EqualValues(t, 0o640, archive.(*Repository).archiveMode)
	assert.True(t, archive.(*Repository).modeExplicit)

	_, statErr := filepath.Glob(filepath.Join(root, "*"))
	require.NoError(t, statErr)
	entries, _ := filepath.Glob(filepath.Join(root, "*"))
	assert.Empty(t, entries, "construction creates no directories")

	_, err = registry.New(context.Background(), "bad", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalDir, Packaging: "tgz", Options: map[string]any{"path": root},
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "a directory repository does not accept archive packaging")
	_, err = registry.New(context.Background(), "bad", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalDir, Permissions: "0600", Options: map[string]any{"path": root},
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = registry.New(context.Background(), "bad", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalArchive, Options: map[string]any{"path": root, "key_pattern": "../{{ .Name }}.tgz"},
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "a key_pattern that escapes the root is rejected before anything is built")
}

func TestRegisterWithVerifier_AdvertisesEvidenceFormats(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithVerifier(&artifacttest.FakeVerifier{}), WithOwner("custom-owner")))
	desc, _ := registry.Lookup(artifact.RepositoryKindLocalArchive)
	assert.Equal(t, []artifact.EvidenceFormat{artifacttest.FakeEvidenceFormat}, desc.Capabilities.EvidenceFormats)

	repo, err := registry.New(context.Background(), "a", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalArchive, Options: map[string]any{"path": t.TempDir()},
	})
	require.NoError(t, err)
	caps := repo.Capabilities()
	assert.Equal(t, desc.Capabilities.EvidenceFormats, caps.EvidenceFormats, "static and constructed capabilities agree")
	assert.Equal(t, "custom-owner", repo.(*Repository).settings.owner)
}

func TestDescriptor_ReflectsHostAndVerifier(t *testing.T) {
	plain := Descriptor(artifact.RepositoryKindLocalDir)
	assert.Empty(t, plain.Capabilities.EvidenceFormats)
	assert.NotNil(t, plain.NewOptions)
	assert.Equal(t, hostEnforcesModes(), plain.Capabilities.PreservesModes)
}

func TestDefaultRegistryRegistration(t *testing.T) {
	artifact.ResetRepositoryKindsForTesting()
	t.Cleanup(artifact.ResetRepositoryKindsForTesting)
	require.NoError(t, Register())
	repo, err := artifact.NewRepository(context.Background(), "x", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()},
	})
	require.NoError(t, err)
	assert.IsType(t, &Repository{}, repo)
	require.ErrorIs(t, Register(), errUtils.ErrArtifactConflict)
}

func TestNewRepository_RejectsBadInputs(t *testing.T) {
	_, err := NewDir("x", nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = NewDir("x", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalArchive, Options: map[string]any{"path": t.TempDir()}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "a dir constructor refuses another kind's configuration")
	_, err = NewArchive("x", &schema.ArtifactRepositoryConfig{Options: map[string]any{"path": ""}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = NewArchive("x", &schema.ArtifactRepositoryConfig{Options: map[string]any{"path": t.TempDir(), "unknown": 1}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)

	repo, err := NewDir("x", &schema.ArtifactRepositoryConfig{Options: map[string]any{"path": "relative/root"}})
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(repo.Root()), "a relative root is made absolute at construction")
}
