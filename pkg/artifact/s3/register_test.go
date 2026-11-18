package s3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/schema"
)

func s3Config(options map[string]any) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Options: options}
}

func TestRegisterWith_RegistersTheKind(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithVerifier(&artifacttest.FakeVerifier{})))
	assert.Equal(t, []string{artifact.RepositoryKindS3}, Kinds())
	assert.True(t, registry.IsRegistered(artifact.RepositoryKindS3))

	desc, ok := registry.Lookup(artifact.RepositoryKindS3)
	require.True(t, ok)
	assert.Equal(t, artifact.DeleteObject, desc.Capabilities.Delete, "the registered descriptor carries the backend's real capabilities")
	assert.True(t, desc.Capabilities.ConditionalUpdate)
	assert.Equal(t, []artifact.LockScope{artifact.LockScopeHost, artifact.LockScopeRepository}, desc.Capabilities.LockScopes)
	assert.Contains(t, desc.Capabilities.Packagings, artifact.PackagingNone)
	assert.Equal(t, artifact.PackagingTgz, desc.DefaultPackaging)
	assert.Equal(t, []artifact.EvidenceFormat{artifacttest.FakeEvidenceFormat}, desc.Capabilities.EvidenceFormats)
	assert.True(t, desc.Capabilities.EnumerateCollection)

	require.ErrorIs(t, RegisterWith(registry), errUtils.ErrArtifactConflict, "double registration is an error, not a silent replacement")
}

func TestDefaultRegistryRegistration(t *testing.T) {
	// The process-wide registry may already hold the kind when other tests registered it first, so
	// only the explicit call is checked: it either registers or reports the duplicate.
	err := Register()
	if err != nil {
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	}
	assert.True(t, artifact.DefaultRepositoryRegistry().IsRegistered(artifact.RepositoryKindS3))
}

func TestDescriptor_NeedsNoIO(t *testing.T) {
	plain := Descriptor()
	assert.Empty(t, plain.Capabilities.EvidenceFormats)
	assert.True(t, plain.Capabilities.ConditionalUpdate)
	signed := Descriptor(WithVerifier(&artifacttest.FakeVerifier{}))
	assert.Len(t, signed.Capabilities.EvidenceFormats, 1)

	spec := noneSpec().fresh(t)
	spec.storeOpts = nil
	conditionless := Descriptor(WithStore(newStoreFor(t, spec.with(func(s *rspec) {}))))
	assert.True(t, conditionless.Capabilities.ConditionalUpdate)
	assert.Empty(t, spec.fake.Requests(), "describing a kind touches no endpoint")
}

func TestRegisteredFactory_ConstructsWithoutIO(t *testing.T) {
	fake := s3test.NewServer(t)
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry))
	repo, err := registry.New(context.Background(), "shared", &schema.ArtifactRepositoryConfig{
		Kind: artifact.RepositoryKindS3, Packaging: "zip",
		Options: map[string]any{
			"bucket": fake.Bucket(), "prefix": "a/b", "region": "us-east-2", "endpoint": fake.Endpoint(), "path_style": true,
			"tags": map[string]any{"team": "platform"}, "key_pattern": "{{ .Name }}/{{ .Version }}.zip",
		},
	})
	require.NoError(t, err)
	s3repo, ok := repo.(*Repository)
	require.True(t, ok)
	assert.Equal(t, artifact.PackagingZip, s3repo.packaging)
	assert.Equal(t, "s3://"+fake.Bucket()+"/a/b", s3repo.Location())
	assert.Equal(t, map[string]string{"team": "platform"}, s3repo.tags)
	caps := repo.Capabilities()
	assert.True(t, caps.HasPackaging(artifact.PackagingZip))
	assert.True(t, caps.HasLockScope(artifact.LockScopeRepository))
	assert.Empty(t, fake.Requests(), "construction and capability description touch no endpoint")
}

func TestNewRepository_RejectsBadInputs(t *testing.T) {
	tests := []struct {
		name string
		cfg  *schema.ArtifactRepositoryConfig
	}{
		{name: "nil config"},
		{name: "wrong kind", cfg: &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"bucket": "acme-artifacts"}}},
		{name: "missing bucket", cfg: s3Config(map[string]any{})},
		{name: "unknown option", cfg: s3Config(map[string]any{"bucket": "acme-artifacts", "buckt": "typo"})},
		{name: "credential-looking option", cfg: s3Config(map[string]any{"bucket": "acme-artifacts", "secret_key": "x"})},
		{name: "bad endpoint", cfg: s3Config(map[string]any{"bucket": "acme-artifacts", "endpoint": "localhost:4566"})},
		{name: "unsupported packaging", cfg: &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Packaging: "rar", Options: map[string]any{"bucket": "acme-artifacts"}}},
		{name: "permissions are local-only", cfg: &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Permissions: "0600", Options: map[string]any{"bucket": "acme-artifacts"}}},
		{name: "reserved prefix", cfg: s3Config(map[string]any{"bucket": "acme-artifacts", "prefix": "x/.atmos-artifacts"})},
		{name: "too many tags", cfg: s3Config(map[string]any{"bucket": "acme-artifacts", "tags": map[string]any{"1": "a", "2": "a", "3": "a", "4": "a", "5": "a", "6": "a", "7": "a", "8": "a", "9": "a", "10": "a", "11": "a"}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := New("bad", tt.cfg)
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Nil(t, repo)
		})
	}
}

// The registered factory returns a nil interface, not a typed nil, on failure.
func TestRegisteredFactory_ReturnsNilInterfaceOnFailure(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry))
	repo, err := registry.New(context.Background(), "bad", s3Config(map[string]any{}))
	require.Error(t, err)
	assert.True(t, repo == nil)
}
