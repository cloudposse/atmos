package oci

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
)

// Inspecting, pinning, listing, and showing a document must never fetch a payload layer. A fresh
// repository (empty caches) makes every request visible to the recorder.
func TestMetadataAndDocumentReadsNeverFetchThePayload(t *testing.T) {
	reg := newRegistry(t, nil)
	writer := newRegistryRepo(t, reg)
	result := publish(t, writer, publishSpec{content: "budget payload"})
	payload := payloadDigest(t, result)

	reader := newRegistryRepo(t, reg)
	reg.recorder.reset()
	ctx := context.Background()

	pinned, err := reader.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)
	inspection, err := reader.Inspect(ctx, pinned)
	require.NoError(t, err)
	require.NotNil(t, inspection.Record)
	stream, err := reader.OpenDocument(ctx, pinned, artifact.DocumentReadme)
	require.NoError(t, err)
	readme, err := io.ReadAll(stream.Reader)
	require.NoError(t, err)
	require.NoError(t, stream.Reader.Close())
	assert.Equal(t, "readme for "+appName+"\n", string(readme))
	_, err = reader.Pin(ctx, artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)
	page, err := reader.List(ctx, artifact.ListQuery{Name: appName})
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)

	assert.Zero(t, reg.recorder.count("GET", payload.Encoded()), "no GET of the payload layer")
	assert.Zero(t, reg.recorder.count("HEAD", payload.Encoded()), "no existence probe of the payload layer")
	assert.Equal(t, 1, reg.recorder.count("GET", "/blobs/"+inspection.Representation.Documents[0].Digest.String()), "the selected document is fetched once")
	assert.Zero(t, reg.recorder.count("GET", "/blobs/"+inspection.Representation.Documents[1].Digest.String()), "the unselected document is not fetched")
	assert.Equal(t, 1, reg.recorder.count("GET", "/blobs/"+pinned.RecordDigest.String()), "the Record config is fetched once and then cached")
	t.Logf("requests for resolve, inspect, show, pin, and list: %d", reg.recorder.total())

	reg.recorder.reset()
	content, err := reader.Read(ctx, pinned)
	require.NoError(t, err)
	assert.Zero(t, reg.recorder.count("GET", payload.Encoded()), "opening streams is lazy")
	for index := range content.Streams {
		if content.Streams[index].Role == artifact.BlobPayload {
			_, err = io.Copy(io.Discard, content.Streams[index].Reader)
			require.NoError(t, err)
		}
	}
	require.NoError(t, content.Close())
	assert.Equal(t, 1, reg.recorder.count("GET", payload.Encoded()), "reading the payload fetches it")
}

func TestForeignInspectionNeverFetchesLayers(t *testing.T) {
	reg := newRegistry(t, nil)
	image := pushImage(t, reg, testPrefix+"/tools/img", "v1")
	repo := newRegistryRepo(t, reg)
	reg.recorder.reset()

	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: "tools/img", Version: "v1"})
	require.NoError(t, err)
	inspection, err := repo.Inspect(context.Background(), pinned)
	require.NoError(t, err)
	assert.Nil(t, inspection.Record, "a foreign image has no Atmos config blob")
	require.NotNil(t, inspection.Representation.OriginalRoot)
	assert.Equal(t, image.manifest, inspection.Representation.OriginalRoot.Digest)
	assert.Equal(t, artifact.PackagingOCIGraph, inspection.Representation.Packaging)
	for _, layer := range image.layers {
		assert.Zero(t, reg.recorder.count("GET", layer.Encoded()), "layers are not fetched to inspect")
	}
	assert.Zero(t, reg.recorder.count("GET", image.config.Encoded()), "a foreign config blob is not read either")
}
