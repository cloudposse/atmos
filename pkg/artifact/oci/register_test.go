package oci

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Identity: "id", Options: nil}

func TestRegisterAddsBothKindsWithRefinedDescriptors(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithTransport(newRegistry(t, nil).transport)))
	assert.True(t, registry.IsRegistered(artifact.RepositoryKindOCIRegistry))
	assert.True(t, registry.IsRegistered(artifact.RepositoryKindOCILayout))
	assert.Equal(t, []string{artifact.RepositoryKindOCIRegistry, artifact.RepositoryKindOCILayout}, Kinds())

	err := RegisterWith(registry)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "double registration is caught, not replaced")

	registered, ok := registry.Lookup(artifact.RepositoryKindOCIRegistry)
	require.True(t, ok)
	assert.Equal(t, artifact.DeleteManifest, registered.Capabilities.Delete)
	assert.True(t, registered.AllowsCatalog)
	layout, ok := registry.Lookup(artifact.RepositoryKindOCILayout)
	require.True(t, ok)
	assert.Equal(t, artifact.DeleteReference, layout.Capabilities.Delete)
	assert.True(t, layout.UsesLocalPath(artifact.OptionPath))

	withVerifier := Descriptor(artifact.RepositoryKindOCILayout, WithVerifier(&fakeFormats{}))
	assert.Equal(t, []artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle}, withVerifier.Capabilities.EvidenceFormats)
	assert.Empty(t, Descriptor(artifact.RepositoryKindOCILayout).Capabilities.EvidenceFormats)
	assert.False(t, Descriptor(artifact.RepositoryKindOCIRegistry, WithoutCollectionEnumeration()).Capabilities.EnumerateCollection)
	assert.False(t, Descriptor(artifact.RepositoryKindOCIRegistry, WithTransport(withoutCatalog{})).Capabilities.EnumerateCollection)
}

// fakeFormats is a verifier that only declares a format.
type fakeFormats struct{ artifact.Verifier }

func (*fakeFormats) Formats() []artifact.EvidenceFormat {
	return []artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle}
}

func TestDefaultRegistryRegistrationIsExplicit(t *testing.T) {
	artifact.ResetRepositoryKindsForTesting()
	t.Cleanup(artifact.ResetRepositoryKindsForTesting)
	assert.False(t, artifact.DefaultRepositoryRegistry().IsRegistered(artifact.RepositoryKindOCILayout), "nothing registers from init")
	require.NoError(t, Register())
	assert.True(t, artifact.DefaultRepositoryRegistry().IsRegistered(artifact.RepositoryKindOCILayout))
}

func TestFactoryValidatesConfigurationBeforeAnyIO(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t, nil)
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithTransport(reg.transport)))

	tests := []struct {
		name string
		cfg  *schema.ArtifactRepositoryConfig
	}{
		{"unknown option", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "ghcr.io", "regstry": "typo"}}},
		{"missing registry", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"prefix": "acme"}}},
		{"registry with a scheme", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "https://ghcr.io"}}},
		{"unsafe prefix", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": "ghcr.io", "prefix": "../escape"}}},
		{"layout without a path", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCILayout, Options: map[string]any{}}},
		{"unsupported packaging", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCILayout, Packaging: "none", Options: map[string]any{"path": t.TempDir()}}},
		{"permissions apply to local archives only", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCILayout, Permissions: "0600", Options: map[string]any{"path": t.TempDir()}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.New(ctx, "bad", tt.cfg)
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
		})
	}

	_, err := NewRepository(ctx, "wrong", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir}, WithOwner("x"))
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "a kind the package does not implement")
	_, err = NewRepository(ctx, "nil", nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	assert.Zero(t, reg.recorder.total(), "validation touched no network")
}

func TestIdentityNeedsATransportFactoryAndIsHandedTheResolver(t *testing.T) {
	ctx := context.Background()
	cfg := registryConfig("registry.example.com")
	cfg.Identity = "registry-publisher"

	_, err := NewRepository(ctx, "bound", cfg, WithTransport(newRegistry(t, nil).transport))
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "an identity is never silently ignored")
	assert.Contains(t, err.Error(), "WithTransportFactory")

	reg := newRegistry(t, nil)
	var gotAlias, gotIdentity string
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithTransportFactory(func(_ context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, resolve artifact.IdentityResolver) (atmosoci.Transport, error) {
		gotAlias, gotIdentity = alias, cfg.Identity
		require.NotNil(t, resolve)
		return reg.transport, nil
	})))
	resolver := func(context.Context, string) (any, error) { return nil, nil }
	opened, err := registry.New(ctx, "bound", cfg, artifact.WithIdentityResolver(resolver))
	require.NoError(t, err)
	assert.Equal(t, "bound", gotAlias)
	assert.Equal(t, "registry-publisher", gotIdentity)
	assert.Zero(t, reg.recorder.total(), "constructing a bound repository resolves no credentials and makes no request")
	assert.NotNil(t, opened)
}

func TestLayoutDirectoryIsNotCreatedByReadsOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "never", "created")
	repo := newLayoutRepoAt(t, path)
	_, err := repo.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	page, err := repo.List(ctx, artifact.ListQuery{})
	require.NoError(t, err)
	assert.Empty(t, page.Entries, "a layout with no references lists nothing, honestly")
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "reads and listing create no destination directory")

	publish(t, repo, publishSpec{content: "created on write"})
	_, err = os.Stat(filepath.Join(path, "index.json"))
	require.NoError(t, err)
}

func TestRelativeLayoutPathsBecomeAbsolute(t *testing.T) {
	repo, err := NewRepository(context.Background(), "rel", layoutConfig("relative/layout"), WithLockDir(t.TempDir()))
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(repo.be.(*layoutBackend).root))
}

// The package keeps the registry client library and Sigstore out of its own imports: it speaks to
// registries only through pkg/oci and to cryptography only through the injected verifier.
func TestPackageDependencyBoundary(t *testing.T) {
	forbidden := []string{
		"github.com/google/go-containerregistry",
		"oras.land/oras-go",
		"github.com/sigstore",
		"github.com/aws/aws-sdk-go-v2",
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/plugin",
		"github.com/cloudposse/atmos/pkg/ci",
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		require.NoError(t, err)
		checked++
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			for _, prefix := range forbidden {
				require.Falsef(t, path == prefix || strings.HasPrefix(path, prefix+"/"), "%s imports forbidden package %s", file, path)
			}
		}
	}
	require.Positive(t, checked, "no source files were checked")
}

// The plugin-format constants are repeated here to avoid importing plugin packages; they must not
// drift from the ones the plugin package writes.
func TestPluginFormatConstantsMatchThePluginPackage(t *testing.T) {
	assert.Equal(t, string(pluginoci.ConfigMediaType), pluginConfigMediaType)
	assert.Equal(t, string(pluginoci.BundleMediaType), pluginBundleMediaType)

	root, err := layoutRoot(pluginFixture, "")
	require.NoError(t, err)
	assert.Equal(t, pluginRoot, root.Digest)
	annotations := fixtureIndexAnnotations(t)
	for _, key := range []string{pluginAnnotationName, pluginAnnotationVersion, pluginAnnotationManifest} {
		assert.Contains(t, annotations, key, "the committed release carries %s", key)
		assert.True(t, strings.HasPrefix(key, pluginPrefix))
	}
}

// fixtureIndexAnnotations reads the release annotations of the committed plugin index.
func fixtureIndexAnnotations(t *testing.T) map[string]string {
	t.Helper()
	store, err := atmosoci.OpenLayoutReadOnly(context.Background(), pluginFixture)
	require.NoError(t, err)
	desc, err := store.Resolve(context.Background(), pluginRoot.String())
	require.NoError(t, err)
	loaded, err := loadRoot(context.Background(), store, desc)
	require.NoError(t, err)
	require.Equal(t, classPlugin, loaded.class, "the committed release classifies as a plugin")
	return loaded.index.Annotations
}
