package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"

	errUtils "github.com/cloudposse/atmos/errors"
)

// imageIndex is embedded under a lower-case name so the embedded field does not shadow the
// ImageIndex method of the interface.
type imageIndex = v1.ImageIndex

// countingIndex counts how often its children are listed.
type countingIndex struct {
	imageIndex
	listed *int
}

func (c countingIndex) IndexManifest() (*v1.IndexManifest, error) {
	*c.listed++
	return c.imageIndex.IndexManifest()
}

// sharedParent lists one nested index twice, as a subgraph shared by two parents appears.
type sharedParent struct {
	imageIndex
	manifest *v1.IndexManifest
	raw      []byte
	child    v1.ImageIndex
}

func (p sharedParent) RawManifest() ([]byte, error)              { return p.raw, nil }
func (p sharedParent) IndexManifest() (*v1.IndexManifest, error) { return p.manifest, nil }
func (p sharedParent) ImageIndex(v1.Hash) (v1.ImageIndex, error) { return p.child, nil }

func TestLegacyIndexGraphVisitsASharedIndexOnceAndStopsOnCancel(t *testing.T) {
	child, err := random.Index(64, 1, 1)
	require.NoError(t, err)
	listed := 0
	shared := countingIndex{imageIndex: child, listed: &listed}
	childDesc, err := partial.Descriptor(child)
	require.NoError(t, err)
	manifest := &v1.IndexManifest{SchemaVersion: 2, MediaType: types.OCIImageIndex, Manifests: []v1.Descriptor{*childDesc, *childDesc}}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	parent := sharedParent{imageIndex: empty.Index, manifest: manifest, raw: raw, child: shared}

	graph, err := legacyIndexGraph(t.Context(), parent)
	require.NoError(t, err)
	assert.Equal(t, 1, listed, "a subgraph shared by two parents is traversed once")
	childDigest, err := child.Digest()
	require.NoError(t, err)
	exists, err := graph.Exists(t.Context(), ocispec.Descriptor{Digest: digest.Digest(childDigest.String())})
	require.NoError(t, err)
	assert.True(t, exists)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = legacyIndexGraph(ctx, parent)
	require.ErrorIs(t, err, context.Canceled)
	image, err := random.Image(64, 1)
	require.NoError(t, err)
	_, err = legacyImageGraph(ctx, image)
	require.ErrorIs(t, err, context.Canceled)
}

func TestLayoutTagKeepsMetadataAndDigestRoots(t *testing.T) {
	dir := t.TempDir()
	layout, err := OpenLayout(t.Context(), dir)
	require.NoError(t, err)
	tagged := pushManifest(t, layout, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[],"annotations":{"n":"1"}}`)
	tagged.ArtifactType = "application/vnd.example"
	tagged.Annotations = map[string]string{"org.example.note": "kept"}
	require.NoError(t, layout.Tag(t.Context(), tagged, "release"))
	assert.Equal(t, map[string]string{"org.example.note": "kept"}, tagged.Annotations, "the caller's descriptor is not mutated")

	byDigest := pushManifest(t, layout, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[],"annotations":{"n":"2"}}`)
	require.NoError(t, layout.Tag(t.Context(), byDigest, byDigest.Digest.String()))

	reopened, err := OpenLayout(t.Context(), dir)
	require.NoError(t, err)
	named, err := reopened.Resolve(t.Context(), "release")
	require.NoError(t, err)
	assert.Equal(t, "application/vnd.example", named.ArtifactType)
	assert.Equal(t, "kept", named.Annotations["org.example.note"])
	root, err := reopened.Resolve(t.Context(), byDigest.Digest.String())
	require.NoError(t, err)
	assert.Equal(t, ocispec.MediaTypeImageManifest, root.MediaType, "a digest-only root keeps its manifest type after reopening")
	assert.Equal(t, []string{"release"}, reopened.References(), "a digest root is listed but never named")
}

func TestLayoutStateMatchesThePersistedIndexAfterAFailedWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory write permissions are not enforced for this process")
	}
	dir := t.TempDir()
	layout, err := OpenLayout(t.Context(), dir)
	require.NoError(t, err)
	desc := pushManifest(t, layout, `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[]}`)

	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	require.Error(t, layout.Tag(t.Context(), desc, "release"))
	assert.Empty(t, layout.References(), "a failed index write leaves the in-memory index unchanged")

	require.NoError(t, os.Chmod(dir, 0o755))
	require.NoError(t, layout.Tag(t.Context(), desc, "release"), "the retry repeats the index update")
	assert.Equal(t, []string{"release"}, layout.References())
	raw, err := os.ReadFile(filepath.Join(dir, ocispec.ImageIndexFile))
	require.NoError(t, err)
	var index ocispec.Index
	require.NoError(t, json.Unmarshal(raw, &index))
	require.Len(t, index.Manifests, 1)
	assert.Equal(t, "release", index.Manifests[0].Annotations[ocispec.AnnotationRefName])
}

func pushManifest(t *testing.T, layout *Layout, manifest string) ocispec.Descriptor {
	t.Helper()
	config := []byte("{}")
	configDesc := Descriptor("application/vnd.oci.empty.v1+json", config)
	err := layout.Push(t.Context(), configDesc, bytes.NewReader(config))
	if err != nil {
		require.ErrorContains(t, err, "already exists")
	}
	desc := Descriptor(ocispec.MediaTypeImageManifest, []byte(manifest))
	require.NoError(t, layout.Push(t.Context(), desc, bytes.NewReader([]byte(manifest))))
	return desc
}

func TestPushReferenceRejectsContentThatDoesNotMatchBeforeSending(t *testing.T) {
	// The repository points at a closed port: a request would fail with a network error, not integrity.
	repo, err := remote.NewRepository("127.0.0.1:1/acme/plugin")
	require.NoError(t, err)
	repo.PlainHTTP = true
	store := &registryStore{repo: repo}
	expected := Descriptor(ocispec.MediaTypeImageManifest, []byte(`{"expected":true}`))

	for name, content := range map[string][]byte{
		"different bytes": []byte(`{"expected":false}`),
		"longer":          []byte(`{"expected":true} `),
		"shorter":         []byte(`{}`),
	} {
		t.Run(name, func(t *testing.T) {
			err := store.PushReference(t.Context(), expected, bytes.NewReader(content), "1.0.0")
			require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
		})
	}
	oversized := expected
	oversized.Size = maxPushedManifest + 1
	require.ErrorIs(t, store.PushReference(t.Context(), oversized, bytes.NewReader(nil), "1.0.0"), errUtils.ErrOCIIntegrity)
}

func TestRepositoryConversionKeepsTheInsecureScheme(t *testing.T) {
	insecure, err := name.NewRepository("registry.example.com/acme/plugin", name.Insecure)
	require.NoError(t, err)
	converted := repositoryOf(insecure)
	assert.True(t, converted.Insecure)
	back, err := converted.legacy()
	require.NoError(t, err)
	assert.Equal(t, "http", back.Scheme(), "every operation on the repository keeps plain HTTP")

	secure, err := name.NewRepository("registry.example.com/acme/plugin")
	require.NoError(t, err)
	converted = repositoryOf(secure)
	assert.False(t, converted.Insecure)
	back, err = converted.legacy()
	require.NoError(t, err)
	assert.Equal(t, "https", back.Scheme())
}

func TestReferrersPresentRequiresEveryReferrerAtTheDestination(t *testing.T) {
	src := memory.New()
	dst := memory.New()
	ctx := t.Context()
	config := []byte("{}")
	configDesc := Descriptor("application/vnd.oci.empty.v1+json", config)
	rootRaw := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"` + configDesc.Digest.String() + `","size":2},"layers":[]}`)
	root := Descriptor(ocispec.MediaTypeImageManifest, rootRaw)
	sigRaw, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageManifest, Config: configDesc,
		Layers: []ocispec.Descriptor{}, Subject: &root,
	})
	require.NoError(t, err)
	signature := Descriptor(ocispec.MediaTypeImageManifest, sigRaw)
	for _, store := range []*memory.Store{src, dst} {
		require.NoError(t, store.Push(ctx, configDesc, bytes.NewReader(config)))
		require.NoError(t, store.Push(ctx, root, bytes.NewReader(rootRaw)))
	}
	require.NoError(t, src.Push(ctx, signature, bytes.NewReader(sigRaw)))

	assert.False(t, referrersPresent(ctx, src, dst, root), "the root alone does not complete an extended copy")
	require.NoError(t, dst.Push(ctx, signature, bytes.NewReader(sigRaw)))
	assert.True(t, referrersPresent(ctx, src, dst, root))
}

func TestLayoutConcurrentTagsKeepResolverAndIndexInStep(t *testing.T) {
	const writers = 16
	dir := t.TempDir()
	layout, err := OpenLayout(t.Context(), dir)
	require.NoError(t, err)
	descriptors := make([]ocispec.Descriptor, writers)
	for i := range descriptors {
		descriptors[i] = pushManifest(t, layout, fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[],"annotations":{"n":"%d"}}`, i))
	}

	var wg sync.WaitGroup
	for i := range descriptors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, layout.Tag(t.Context(), descriptors[i], "release"))
		}()
	}
	wg.Wait()

	resolved, err := layout.Resolve(t.Context(), "release")
	require.NoError(t, err)
	reopened, err := OpenLayout(t.Context(), dir)
	require.NoError(t, err)
	persisted, err := reopened.Resolve(t.Context(), "release")
	require.NoError(t, err)
	assert.Equal(t, resolved.Digest, persisted.Digest, "the persisted index names the same manifest the resolver returns")
	assert.Equal(t, []string{"release"}, reopened.References())
}

func TestLegacyGraphSpoolsStreamingLayersAndCleansUp(t *testing.T) {
	payload := "streamed layer content"
	image, err := mutate.AppendLayers(empty.Image, stream.NewLayer(io.NopCloser(strings.NewReader(payload))))
	require.NoError(t, err)

	graph, err := legacyImageGraph(t.Context(), image)
	require.NoError(t, err)
	manifest, err := image.Manifest()
	require.NoError(t, err)
	require.Len(t, manifest.Layers, 1)
	layer := ocispec.Descriptor{Digest: digest.Digest(manifest.Layers[0].Digest.String())}
	var reads [][]byte
	for range 2 {
		reader, err := graph.Fetch(t.Context(), layer)
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.NoError(t, reader.Close())
		reads = append(reads, content)
	}
	assert.NotEmpty(t, reads[0], "the spooled layer holds the streamed bytes")
	assert.Equal(t, reads[0], reads[1], "a streamed layer can be read again after it was spooled")
	assert.Equal(t, layer.Digest, digest.FromBytes(reads[0]), "the spooled bytes match the manifest digest")

	spoolDir := graph.spoolDir
	require.DirExists(t, spoolDir)
	graph.close()
	assert.NoDirExists(t, spoolDir)
	graph.close()
}
