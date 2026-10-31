package oci

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// pluginFixture is the committed plugin release layout captured by the compatibility suite.
var pluginFixture = filepath.Join("..", "compat", "testdata", "plugin-release")

// pluginRoot is the digest of the fixture's release index.
const pluginRoot digest.Digest = "sha256:af34a37555cc97c7f1043fdb2ad3404a062d97d1ce07d96c5a594f6b5ac83b59"

// copyFixture copies the committed layout so no test ever writes to the fixture.
func copyFixture(t *testing.T) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, filepath.WalkDir(pluginFixture, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(pluginFixture, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	}))
	return destination
}

// importPlugin imports the plugin release fixture under ref.
func importPlugin(t *testing.T, repo *Repository, ref artifact.Ref) *artifact.PublishResult {
	t.Helper()
	result, err := repo.ImportLayout(context.Background(), copyFixture(t), ref, ImportOptions{})
	require.NoError(t, err)
	return result
}

func TestPluginReleaseIsImportedByteIdenticallyAndInspectedWithoutAConfigBlob(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
		result := importPlugin(t, repo, ref)
		assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
		assert.Equal(t, pluginRoot, result.Pinned.ResolvedDigest, "the release keeps its original index digest")
		assert.Empty(t, result.Pinned.RecordDigest, "a plugin release has no Atmos Record")

		pinned, err := repo.Resolve(context.Background(), ref)
		require.NoError(t, err)
		inspection, err := repo.Inspect(context.Background(), pinned)
		require.NoError(t, err)
		assert.Nil(t, inspection.Record)
		require.NotNil(t, inspection.Representation.OriginalRoot)
		assert.Equal(t, "example/fixture", inspection.Representation.OriginalRoot.Annotations[pluginAnnotationName])
		assert.Equal(t, artifact.PackagingOCIGraph, inspection.Representation.Packaging)

		_, err = repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
		require.ErrorIs(t, err, errUtils.ErrArtifactDocumentMissing, "a foreign object has no Atmos documents")

		content, err := repo.Read(context.Background(), pinned)
		require.NoError(t, err)
		defer content.Close()
		require.NotNil(t, content.Graph)
		assert.Nil(t, content.Record)
		assert.Equal(t, pluginRoot, content.Graph.Root().Digest)
		nodes := walkGraph(t, content.Graph)
		assert.GreaterOrEqual(t, nodes, 7, "the index, both platform manifests, configs and bundles all transfer")
	})
}

// walkGraph reads every node below the root with verification and returns how many it saw.
func walkGraph(t *testing.T, graph artifact.GraphSource) int {
	t.Helper()
	seen := map[digest.Digest]bool{}
	var visit func(desc ocispec.Descriptor)
	visit = func(desc ocispec.Descriptor) {
		if seen[desc.Digest] {
			return
		}
		seen[desc.Digest] = true
		reader, err := graph.Fetch(context.Background(), desc)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, reader)
		require.NoError(t, err, "node %s verifies against its descriptor", desc.Digest)
		require.NoError(t, reader.Close())
		successors, err := graph.Successors(context.Background(), desc)
		require.NoError(t, err)
		for _, successor := range successors {
			visit(successor)
		}
	}
	visit(graph.Root())
	return len(seen)
}

func TestPluginReleaseExportsToAStandardLayoutAndImportsAgain(t *testing.T) {
	source := newRegistryRepo(t, newRegistry(t, nil))
	ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
	importPlugin(t, source, ref)
	pinned, err := source.Resolve(context.Background(), ref)
	require.NoError(t, err)

	exported := filepath.Join(t.TempDir(), "export")
	root, err := source.ExportLayout(context.Background(), pinned, exported)
	require.NoError(t, err)
	assert.Equal(t, pluginRoot, root.Digest)

	target := newLayoutRepo(t)
	result, err := target.ImportLayout(context.Background(), exported, ref, ImportOptions{})
	require.NoError(t, err)
	assert.Equal(t, pluginRoot, result.Pinned.ResolvedDigest)

	_, err = target.ImportLayout(context.Background(), exported, ref, ImportOptions{Select: "no-such-root"})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
}

func TestPluginReleaseNeverChangesAndIsNeverOverwritten(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ctx := context.Background()
		ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
		result := importPlugin(t, repo, ref)

		again, err := repo.ImportLayout(ctx, copyFixture(t), ref, ImportOptions{ExpectedPrior: &result.Pinned})
		require.NoError(t, err)
		assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome, "an identical re-import is a no-op")

		generic := prepare(t, publishSpec{name: ref.Name, version: ref.Version, content: "generic"})
		for name, options := range map[string]artifact.PublishOptions{
			"no force":    {ExpectedPrior: &result.Pinned},
			"with force":  {ExpectedPrior: &result.Pinned, Force: true},
			"no expected": {Force: true},
		} {
			_, err := repo.Publish(ctx, generic, options)
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict, name)
			assert.Contains(t, err.Error(), "immutable", name)
		}
		pinned, err := repo.Resolve(ctx, ref)
		require.NoError(t, err)
		assert.Equal(t, pluginRoot, pinned.ResolvedDigest, "the release was never mutated")

		_, err = repo.Delete(ctx, pinned, artifact.DeleteOptions{Scope: repo.Capabilities().Delete})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "generic deletion never removes a plugin release")
		assert.True(t, resolves(repo, ref.Name, ref.Version))
	})
}

func TestIncomingPluginReleaseNeverReplacesAGenericArtifact(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
		generic := publish(t, repo, publishSpec{name: ref.Name, version: ref.Version, content: "generic"})

		for _, force := range []bool{false, true} {
			_, err := repo.ImportLayout(context.Background(), copyFixture(t), ref, ImportOptions{ExpectedPrior: &generic.Pinned, Force: force})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
		}
		pinned, err := repo.Resolve(context.Background(), ref)
		require.NoError(t, err)
		assert.Equal(t, generic.Pinned.ResolvedDigest, pinned.ResolvedDigest)
	})
}

func TestGenericArtifactsAndPluginsCoexistUnderOnePrefix(t *testing.T) {
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg)
	plugin := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
	importPlugin(t, repo, plugin)
	publish(t, repo, publishSpec{name: plugin.Name, version: "2.0.0", content: "generic sibling"})
	publish(t, repo, publishSpec{name: "plugins/other", version: "1.0.0", content: "generic neighbor"})

	assert.Equal(t, []string{"plugins/example:2.0.0", "plugins/other:1.0.0"}, listAll(t, repo, artifact.ListQuery{}), "legacy plugin tags are never listed as generic artifacts")
	pinned, err := repo.Resolve(context.Background(), plugin)
	require.NoError(t, err)
	assert.Equal(t, pluginRoot, pinned.ResolvedDigest, "generic publication beside a plugin leaves it untouched")

	removed := mustResolveRef(t, repo, artifact.Ref{Name: "plugins/example", Version: "2.0.0"})
	_, err = repo.Delete(context.Background(), removed, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	assert.True(t, resolves(repo, plugin.Name, plugin.Version), "deleting a generic sibling never touches the plugin tag")
}

func mustResolveRef(t *testing.T, repo *Repository, ref artifact.Ref) artifact.PinnedReference {
	t.Helper()
	pinned, err := repo.Resolve(context.Background(), ref)
	require.NoError(t, err)
	return pinned
}

func TestPluginImmutabilityPolicy(t *testing.T) {
	plugin := &artifact.Inspection{Representation: artifact.Representation{
		OriginalRoot: &ocispec.Descriptor{Annotations: map[string]string{pluginAnnotationName: "x/y", pluginAnnotationVersion: "1"}},
	}}
	bundle := &artifact.Inspection{Representation: artifact.Representation{Payload: []ocispec.Descriptor{{MediaType: pluginBundleMediaType}}}}
	labeled := &artifact.Inspection{Record: &artifact.Record{Metadata: artifact.Metadata{Labels: map[string]string{pluginPrefix + "name": "x"}}}}
	generic := &artifact.Inspection{Representation: artifact.Representation{OriginalRoot: &ocispec.Descriptor{Annotations: map[string]string{"org.example": "x"}}}}
	stagedPlugin := &artifact.PreparedArtifact{Blobs: []artifact.StagedBlob{{Descriptor: ocispec.Descriptor{MediaType: pluginBundleMediaType}}}}
	stagedGeneric := &artifact.PreparedArtifact{Blobs: []artifact.StagedBlob{{Descriptor: ocispec.Descriptor{MediaType: artifact.MediaTypeTar}}}}

	tests := []struct {
		name      string
		candidate service.Candidate
		want      bool
	}{
		{"existing release annotations", service.Candidate{Existing: true, Inspection: plugin}, true},
		{"copy source release", service.Candidate{Inspection: plugin}, true},
		{"bundle layer", service.Candidate{Existing: true, Inspection: bundle}, true},
		{"plugin labels", service.Candidate{Existing: true, Inspection: labeled}, true},
		{"incoming plugin bundle", service.Candidate{Prepared: stagedPlugin}, true},
		{"foreign image", service.Candidate{Existing: true, Inspection: generic}, false},
		{"incoming generic", service.Candidate{Prepared: stagedGeneric}, false},
		{"empty", service.Candidate{}, false},
	}
	policy := PluginImmutability()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			immutable, why := policy.Immutable(tt.candidate)
			assert.Equal(t, tt.want, immutable)
			assert.Equal(t, tt.want, why != "", "an immutable verdict explains itself")
		})
	}
}

// With the service in front, immutability is enforced in both directions, with and without force.
func TestServiceKeepsPluginReleasesImmutableInBothDirections(t *testing.T) {
	ctx := context.Background()
	registryRepo := newRegistryRepo(t, newRegistry(t, nil))
	layoutRepo := newLayoutRepo(t)
	svc, err := service.New(&service.StaticRepositories{Repositories: map[string]artifact.Repository{"reg": registryRepo, "lay": layoutRepo}}, nil,
		service.WithImmutabilityPolicy(PluginImmutability()), service.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)

	ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
	importPlugin(t, registryRepo, ref)
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "a.txt"), []byte("generic"), 0o644))

	// Existing plugin, incoming generic publication.
	for _, force := range []bool{false, true} {
		_, err := svc.PublishDetailed(ctx, artifact.PublishRequest{Repository: "reg", Source: source, Name: ref.Name, Version: ref.Version, Force: force})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	}
	assert.Equal(t, pluginRoot, mustResolveRef(t, registryRepo, ref).ResolvedDigest)

	// Existing generic, incoming plugin release copied from the registry.
	_, err = svc.PublishDetailed(ctx, artifact.PublishRequest{Repository: "lay", Source: source, Name: ref.Name, Version: ref.Version})
	require.NoError(t, err)
	for _, force := range []bool{false, true} {
		_, err := svc.Copy(ctx, artifact.CopyRequest{Ref: ref, From: "reg", To: "lay", Force: force})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
		assert.Contains(t, err.Error(), "immutable")
	}
	assert.NotEqual(t, pluginRoot, mustResolveRef(t, layoutRepo, ref).ResolvedDigest)
}
