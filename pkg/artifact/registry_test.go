package artifact

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeKind registers a kind whose factory records its inputs and returns a mock repository.
type fakeKind struct {
	calls   int
	alias   string
	options RepositoryOptions
	repo    *MockRepository
}

func (f *fakeKind) factory(_ context.Context, alias string, _ *schema.ArtifactRepositoryConfig, opts ...RepositoryOption) (Repository, error) {
	f.calls++
	f.alias = alias
	f.options = ApplyRepositoryOptions(opts...)
	return f.repo, nil
}

func localDirConfig(path string) *schema.ArtifactRepositoryConfig {
	return &schema.ArtifactRepositoryConfig{Kind: RepositoryKindLocalDir, Options: map[string]any{"path": path}}
}

func TestRepositoryRegistry_RegisterRejectsInvalidAndDuplicateKinds(t *testing.T) {
	ctrl := gomock.NewController(t)
	fake := &fakeKind{repo: NewMockRepository(ctrl)}
	desc, ok := BuiltinKind(RepositoryKindLocalDir)
	require.True(t, ok)

	tests := []struct {
		name    string
		desc    *KindDescriptor
		factory RepositoryFactory
		wantErr error
	}{
		{name: "empty kind", desc: &KindDescriptor{}, factory: fake.factory, wantErr: errUtils.ErrArtifactConfigInvalid},
		{name: "malformed kind", desc: &KindDescriptor{Kind: "Not A Kind"}, factory: fake.factory, wantErr: errUtils.ErrArtifactConfigInvalid},
		{name: "nil factory", desc: desc, factory: nil, wantErr: errUtils.ErrArtifactConfigInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRepositoryRegistry().Register(tt.desc, tt.factory)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}

	registry := NewRepositoryRegistry()
	require.NoError(t, registry.Register(desc, fake.factory))
	err := registry.Register(desc, fake.factory)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a duplicate kind must be rejected")
	assert.Contains(t, err.Error(), RepositoryKindLocalDir)
}

func TestRepositoryRegistry_LookupFallsBackToBuiltinAndPrefersRegistered(t *testing.T) {
	registry := NewRepositoryRegistry()

	builtin, ok := registry.Lookup(RepositoryKindS3)
	require.True(t, ok, "V1 kinds validate before any backend registers")
	assert.False(t, registry.IsRegistered(RepositoryKindS3))
	assert.Equal(t, PackagingTgz, builtin.DefaultPackaging)

	_, ok = registry.Lookup("ftp/server")
	assert.False(t, ok)

	refined := *builtin
	refined.DefaultPackaging = PackagingZip
	require.NoError(t, registry.Register(&refined, (&fakeKind{}).factory))
	got, ok := registry.Lookup(RepositoryKindS3)
	require.True(t, ok)
	assert.Equal(t, PackagingZip, got.DefaultPackaging, "a registered descriptor takes precedence")
	assert.True(t, registry.IsRegistered(RepositoryKindS3))
}

func TestRepositoryRegistry_KindsListsBuiltinAndRegisteredSorted(t *testing.T) {
	registry := NewRepositoryRegistry()
	require.NoError(t, registry.Register(&KindDescriptor{Kind: "gcp/gcs"}, (&fakeKind{}).factory))
	assert.Equal(t, []string{"aws/s3", "gcp/gcs", "local/archive", "local/dir", "oci/layout", "oci/registry"}, registry.Kinds())
}

func TestRepositoryRegistry_NewConstructsThroughFactoryWithoutNetworkOrCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockRepository(ctrl)
	repo.EXPECT().Capabilities().Return(Capabilities{Read: true, Write: true, Packagings: []Packaging{PackagingNone}}).AnyTimes()
	fake := &fakeKind{repo: repo}
	registry := NewRepositoryRegistry()
	desc, _ := BuiltinKind(RepositoryKindLocalDir)
	require.NoError(t, registry.Register(desc, fake.factory))

	resolved := 0
	resolver := func(context.Context, string) (any, error) { resolved++; return nil, nil }
	lookup := func(string) (Repository, error) { return nil, nil }
	got, err := registry.New(context.Background(), "local", localDirConfig(t.TempDir()), WithIdentityResolver(resolver), WithRepositoryLookup(lookup))
	require.NoError(t, err)
	assert.Same(t, repo, got)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, "local", fake.alias)
	assert.NotNil(t, fake.options.ResolveIdentity)
	assert.NotNil(t, fake.options.LookupRepository)
	assert.Zero(t, resolved, "constructing a repository must not resolve credentials")
	assert.True(t, got.Capabilities().Write, "capabilities are available immediately")
}

func TestRepositoryRegistry_NewRejectsUnknownUnregisteredAndInvalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	fake := &fakeKind{repo: NewMockRepository(ctrl)}
	registry := NewRepositoryRegistry()
	desc, _ := BuiltinKind(RepositoryKindLocalDir)
	require.NoError(t, registry.Register(desc, fake.factory))

	_, err := registry.New(context.Background(), "x", &schema.ArtifactRepositoryConfig{Kind: "ftp/server"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Contains(t, err.Error(), "unknown kind")

	_, err = registry.New(context.Background(), "x", &schema.ArtifactRepositoryConfig{Kind: RepositoryKindS3, Options: map[string]any{"bucket": "acme-artifacts"}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a known V1 kind without a registered backend is unsupported, not unknown")

	_, err = registry.New(context.Background(), "x", &schema.ArtifactRepositoryConfig{Kind: RepositoryKindLocalDir})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "missing required options fail before the factory runs")

	_, err = registry.New(context.Background(), "x", nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Zero(t, fake.calls, "invalid configuration never reaches the factory")
}

func TestRepositoryRegistry_NewWrapsFactoryErrors(t *testing.T) {
	registry := NewRepositoryRegistry()
	desc, _ := BuiltinKind(RepositoryKindLocalDir)
	boom := errors.New("boom")
	require.NoError(t, registry.Register(desc, func(context.Context, string, *schema.ArtifactRepositoryConfig, ...RepositoryOption) (Repository, error) {
		return nil, boom
	}))
	_, err := registry.New(context.Background(), "local", localDirConfig(t.TempDir()))
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), `repository "local"`)
}

func TestRepositoryRegistry_ResetClearsRegistrations(t *testing.T) {
	registry := NewRepositoryRegistry()
	desc, _ := BuiltinKind(RepositoryKindLocalDir)
	require.NoError(t, registry.Register(desc, (&fakeKind{}).factory))
	registry.Reset()
	assert.False(t, registry.IsRegistered(RepositoryKindLocalDir))
	require.NoError(t, registry.Register(desc, (&fakeKind{}).factory), "a reset registry accepts the kind again")
}

func TestRepositoryRegistry_ConcurrentRegistrationIsSafe(t *testing.T) {
	registry := NewRepositoryRegistry()
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Every goroutine races for the same kind; exactly one must win.
			errs[i] = registry.Register(&KindDescriptor{Kind: "race/kind"}, (&fakeKind{}).factory)
			_ = registry.Kinds()
			_, _ = registry.Lookup("race/kind")
		}()
	}
	wg.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
			continue
		}
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	}
	assert.Equal(t, 1, winners)
}

func TestDefaultRegistry_RegisterNewAndReset(t *testing.T) {
	t.Cleanup(ResetRepositoryKindsForTesting)
	ResetRepositoryKindsForTesting()
	ctrl := gomock.NewController(t)
	fake := &fakeKind{repo: NewMockRepository(ctrl)}
	desc, _ := BuiltinKind(RepositoryKindLocalDir)

	require.NoError(t, RegisterRepositoryKind(desc, fake.factory))
	require.ErrorIs(t, RegisterRepositoryKind(desc, fake.factory), errUtils.ErrArtifactConflict)
	assert.Panics(t, func() { MustRegisterRepositoryKind(desc, fake.factory) })

	got, err := NewRepository(context.Background(), "local", localDirConfig(t.TempDir()))
	require.NoError(t, err)
	assert.Same(t, fake.repo, got)
	_, ok := LookupRepositoryKind(RepositoryKindLocalDir)
	assert.True(t, ok)
	assert.Contains(t, RepositoryKinds(), RepositoryKindLocalDir)

	ResetRepositoryKindsForTesting()
	_, err = NewRepository(context.Background(), "local", localDirConfig(t.TempDir()))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestApplyRepositoryOptions_IgnoresNilOptions(t *testing.T) {
	applied := ApplyRepositoryOptions(nil, WithRepositoryLookup(func(alias string) (Repository, error) {
		return nil, fmt.Errorf("lookup %s", alias)
	}))
	require.NotNil(t, applied.LookupRepository)
	_, err := applied.LookupRepository("x")
	require.EqualError(t, err, "lookup x")
}
