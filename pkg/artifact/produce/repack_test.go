package produce_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
)

// treeOf materializes a prepared artifact's per-file payload into a directory.
func treeOf(t *testing.T, prepared *artifact.PreparedArtifact) string {
	t.Helper()
	tree := t.TempDir()
	blobs := payloadBlobs(prepared)
	for index := range blobs {
		fixtures.WriteFile(t, tree, blobs[index].Path, string(readBlob(t, &blobs[index])), 0o644)
	}
	return tree
}

func documentsOf(t *testing.T, prepared *artifact.PreparedArtifact) map[artifact.DocumentRole][]byte {
	t.Helper()
	docs := map[artifact.DocumentRole][]byte{}
	for index := range prepared.Blobs {
		if prepared.Blobs[index].Role == artifact.BlobDocument {
			docs[prepared.Blobs[index].DocumentRole] = readBlob(t, &prepared.Blobs[index])
		}
	}
	return docs
}

func TestRepack_PreservesRecordAndMatchesFreshArchive(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	original, _ := prepare(t, request(src, artifact.PackagingNone))
	fresh, _ := prepare(t, request(src, artifact.PackagingTgz))

	req := &produce.RepackRequest{
		Record:      original.Record,
		Tree:        treeOf(t, original),
		Documents:   documentsOf(t, original),
		Packaging:   artifact.PackagingTgz,
		OperationID: "copy-op",
		PreparedAt:  fixedTime.AddDate(0, 0, 3),
	}
	repacked, err := produce.New(produce.WithStagingRoot(t.TempDir())).Repack(context.Background(), req)
	require.NoError(t, err)
	defer repacked.Close()

	originalBytes, err := original.Record.Canonical()
	require.NoError(t, err)
	repackedBytes, err := repacked.Record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, originalBytes, repackedBytes, "a copy preserves Record bytes")
	assert.Equal(t, original.IntentDigest, repacked.IntentDigest)
	assert.Equal(t, "copy-op", repacked.OperationID)

	repackedPayload := payloadBlobs(repacked)
	freshPayload := payloadBlobs(fresh)
	require.Len(t, repackedPayload, 1)
	assert.Equal(t, readBlob(t, &freshPayload[0]), readBlob(t, &repackedPayload[0]), "deterministic assembly yields identical archive bytes")
}

func TestRepack_Failures(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	original, _ := prepare(t, request(src, artifact.PackagingNone))
	good := func() *produce.RepackRequest {
		return &produce.RepackRequest{
			Record: original.Record, Tree: treeOf(t, original), Documents: documentsOf(t, original),
			Packaging: artifact.PackagingTar, OperationID: "op", PreparedAt: fixedTime,
		}
	}
	tests := []struct {
		name   string
		mutate func(*produce.RepackRequest)
		want   error
	}{
		{"no operation", func(r *produce.RepackRequest) { r.OperationID = "" }, errUtils.ErrArtifactConfigInvalid},
		{"missing document", func(r *produce.RepackRequest) { delete(r.Documents, artifact.DocumentReadme) }, errUtils.ErrArtifactDocumentMissing},
		{"tampered document", func(r *produce.RepackRequest) { r.Documents[artifact.DocumentReadme] = []byte("tampered") }, errUtils.ErrArtifactIntegrityFailed},
		{"tampered tree file", func(r *produce.RepackRequest) {
			require.NoError(t, os.WriteFile(filepath.Join(r.Tree, "nested", "deep", "file.txt"), []byte("tampered content"), 0o644))
		}, errUtils.ErrArtifactPackagingInvalid},
		{"missing tree file", func(r *produce.RepackRequest) {
			require.NoError(t, os.Remove(filepath.Join(r.Tree, "empty.txt")))
		}, errUtils.ErrArtifactPackagingInvalid},
		{"invalid record", func(r *produce.RepackRequest) { r.Record.ContentDigest = "" }, errUtils.ErrArtifactRecordInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := good()
			tt.mutate(req)
			staging := t.TempDir()
			_, err := produce.New(produce.WithStagingRoot(staging)).Repack(context.Background(), req)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			items, readErr := os.ReadDir(staging)
			require.NoError(t, readErr)
			assert.Empty(t, items)
		})
	}
}
