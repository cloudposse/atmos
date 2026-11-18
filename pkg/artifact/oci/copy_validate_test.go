package oci

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// rewriteLayoutManifest replaces the single root manifest of an exported layout with a mutated
// copy, writing the new manifest blob and pointing the index at it. It returns the new root digest.
func rewriteLayoutManifest(t *testing.T, dir string, mutate func(*ocispec.Manifest)) digest.Digest {
	t.Helper()
	indexPath := filepath.Join(dir, ocispec.ImageIndexFile)
	rawIndex, err := os.ReadFile(indexPath)
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(rawIndex, &index))
	require.Len(t, index.Manifests, 1)
	entry := &index.Manifests[0]

	blobDir := filepath.Join(dir, ocispec.ImageBlobsDir, string(entry.Digest.Algorithm()))
	rawManifest, err := os.ReadFile(filepath.Join(blobDir, entry.Digest.Encoded()))
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(rawManifest, &manifest))
	mutate(&manifest)
	rewritten, err := json.Marshal(manifest)
	require.NoError(t, err)

	entry.Digest = digest.FromBytes(rewritten)
	entry.Size = int64(len(rewritten))
	require.NoError(t, os.WriteFile(filepath.Join(blobDir, entry.Digest.Encoded()), rewritten, 0o600))
	rawIndex, err = json.Marshal(index)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(indexPath, rawIndex, 0o600))
	return entry.Digest
}

// exportedPackage publishes a package to a layout and exports it, returning the export directory.
func exportedPackage(t *testing.T) (dir string, published artifact.PinnedReference) {
	t.Helper()
	source := newLayoutRepo(t)
	published = publish(t, source, publishSpec{content: "exported content"}).Pinned
	dir = t.TempDir()
	_, err := source.ExportLayout(context.Background(), published, dir)
	require.NoError(t, err)
	return dir, published
}

// A layout holds a canonical, correctly named Record yet may lack the archive payload layer. A
// forced import of it must be rejected before the destination tag moves: the existing package
// stays readable instead of being replaced by one that every later read rejects.
func TestImportLayout_RejectsAMalformedPackageBeforeReplacingTheTag(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*ocispec.Manifest)
	}{
		{name: "no archive payload layer", mutate: func(m *ocispec.Manifest) {
			var kept []ocispec.Descriptor
			for _, layer := range m.Layers {
				if _, isPayload := packagingOf(layer.MediaType); !isPayload {
					kept = append(kept, layer)
				}
			}
			m.Layers = kept
		}},
		{name: "two archive payload layers", mutate: func(m *ocispec.Manifest) {
			for _, layer := range m.Layers {
				if _, isPayload := packagingOf(layer.MediaType); isPayload {
					m.Layers = append(m.Layers, layer)
					return
				}
			}
		}},
		{name: "a document the Record does not describe", mutate: func(m *ocispec.Manifest) {
			m.Layers = m.Layers[:len(m.Layers)-1]
		}},
		{name: "a layer that is neither payload nor document", mutate: func(m *ocispec.Manifest) {
			m.Layers = append(m.Layers, ocispec.Descriptor{MediaType: "application/x-unknown", Digest: digest.FromString("x"), Size: 1})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := exportedPackage(t)
			rewriteLayoutManifest(t, dir, tt.mutate)

			destination := newLayoutRepo(t)
			existing := publish(t, destination, publishSpec{content: "already published"}).Pinned
			ref := artifact.Ref{Name: appName, Version: appVersion}
			before, err := destination.be.(*layoutBackend).references(ctx)
			require.NoError(t, err)

			_, err = destination.ImportLayout(ctx, dir, ref, ImportOptions{Force: true, ExpectedPrior: &existing})
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)

			after, err := destination.be.(*layoutBackend).references(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after, "the rejected import changed no tag")
			intact, err := destination.Resolve(ctx, ref)
			require.NoError(t, err)
			assert.Equal(t, existing.ResolvedDigest, intact.ResolvedDigest, "the existing package is still the one served")
			_, err = destination.Inspect(ctx, intact)
			require.NoError(t, err, "the existing package still inspects")
		})
	}
}

// An import destination is a name:version tag. A digest-only reference is valid to resolve with,
// but a layout would store it under an empty version that artifact listing never exposes, so the
// import refuses it before anything is written to the destination.
func TestImportLayout_RequiresAVersionForTheDestination(t *testing.T) {
	ctx := context.Background()
	dir, published := exportedPackage(t)

	tests := []struct {
		name string
		ref  artifact.Ref
	}{
		{name: "digest only", ref: artifact.Ref{Name: appName, Digest: published.ResolvedDigest}},
		{name: "neither version nor digest", ref: artifact.Ref{Name: appName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destinationPath := filepath.Join(t.TempDir(), "layout")
			destination := newLayoutRepoAt(t, destinationPath)

			_, err := destination.ImportLayout(ctx, dir, tt.ref, ImportOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)

			_, statErr := os.Stat(destinationPath)
			assert.True(t, os.IsNotExist(statErr), "the destination layout was never opened for writing")
		})
	}

	t.Run("a version imports and lists", func(t *testing.T) {
		destination := newLayoutRepo(t)
		_, err := destination.ImportLayout(ctx, dir, artifact.Ref{Name: appName, Version: appVersion}, ImportOptions{})
		require.NoError(t, err)
		refs, err := destination.be.(*layoutBackend).references(ctx)
		require.NoError(t, err)
		assert.Equal(t, []artifact.Ref{{Name: appName, Version: appVersion}}, refs)
	})
}

// A native copy between repositories shares the transfer with an import, so it too refuses a
// package without its archive payload layer before the destination tag moves.
func TestCopyFrom_RejectsAMalformedPackageBeforeReplacingTheTag(t *testing.T) {
	ctx := context.Background()
	dir, _ := exportedPackage(t)
	rewritten := rewriteLayoutManifest(t, dir, func(m *ocispec.Manifest) {
		var kept []ocispec.Descriptor
		for _, layer := range m.Layers {
			if _, isPayload := packagingOf(layer.MediaType); !isPayload {
				kept = append(kept, layer)
			}
		}
		m.Layers = kept
	})
	source := newLayoutRepoAt(t, dir)
	destination := newLayoutRepo(t)
	existing := publish(t, destination, publishSpec{content: "already published"}).Pinned
	ref := artifact.Ref{Name: appName, Version: appVersion}

	_, err := destination.CopyFrom(ctx, source, artifact.PinnedReference{Ref: ref, ResolvedDigest: rewritten}, artifact.CopyFromOptions{Force: true, ExpectedPrior: &existing})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)

	intact, err := destination.Resolve(ctx, ref)
	require.NoError(t, err)
	assert.Equal(t, existing.ResolvedDigest, intact.ResolvedDigest, "the existing package is still the one served")
}
