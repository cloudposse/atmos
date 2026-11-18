package produce_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
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

// TestRepack_PreparedArtifactIsIndependentOfTheRequest proves that what the caller passes in is
// copied at the boundary: editing the request after Repack returns changes nothing the prepared
// artifact serves, and editing the prepared artifact changes nothing in the request.
func TestRepack_PreparedArtifactIsIndependentOfTheRequest(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	original, _ := prepare(t, request(src, artifact.PackagingNone))
	wantDocs := documentsOf(t, original)
	require.NotEmpty(t, wantDocs[artifact.DocumentReadme])

	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			documents := documentsOf(t, original)
			req := &produce.RepackRequest{
				Record: original.Record.Clone(), Tree: treeOf(t, original), Documents: documents,
				Packaging: packaging, OperationID: "op", PreparedAt: fixedTime,
			}
			repacked, err := produce.New(produce.WithStagingRoot(t.TempDir())).Repack(context.Background(), req)
			require.NoError(t, err)
			defer repacked.Close()
			wantRecord, err := repacked.Record.Canonical()
			require.NoError(t, err)

			// Request to prepared artifact: scribble over everything the caller still owns.
			for role := range req.Documents {
				for index := range req.Documents[role] {
					req.Documents[role][index] = 'X'
				}
			}
			req.Record.Metadata.Labels = map[string]string{"scribble": "x"}
			req.Record.Inventory[0].Path = "scribble"
			req.Record.Inventory[len(req.Record.Inventory)-1].Digest = "sha256:scribble"
			req.Record.Documents[0].SourcePath = "scribble"

			for role, want := range wantDocs {
				blob := documentBlob(t, repacked, role)
				got := readBlob(t, blob)
				assert.Equal(t, want, got, "the %s document served later is the one sealed in its descriptor", role)
				assert.Equal(t, blob.Descriptor.Digest, digest.FromBytes(got))
			}
			gotRecord, err := repacked.Record.Canonical()
			require.NoError(t, err)
			assert.Equal(t, wantRecord, gotRecord, "the prepared Record does not follow the request's Record")
			if packaging == artifact.PackagingNone {
				wantFiles := map[string][]byte{}
				for index, blob := range payloadBlobs(original) {
					wantFiles[blob.Path] = readBlob(t, &payloadBlobs(original)[index])
				}
				gotBlobs := payloadBlobs(repacked)
				require.Len(t, gotBlobs, len(wantFiles))
				for index := range gotBlobs {
					assert.Equal(t, wantFiles[gotBlobs[index].Path], readBlob(t, &gotBlobs[index]), "payload files still verify against the original inventory: %s", gotBlobs[index].Path)
				}
			}

			// Prepared artifact to request: editing the result must not reach the request.
			req.Record.Inventory[0].Path = "kept"
			repacked.Record.Inventory[0].Path = "scribble-too"
			assert.Equal(t, "kept", req.Record.Inventory[0].Path)
		})
	}
}

func documentBlob(t *testing.T, prepared *artifact.PreparedArtifact, role artifact.DocumentRole) *artifact.StagedBlob {
	t.Helper()
	for index := range prepared.Blobs {
		if prepared.Blobs[index].Role == artifact.BlobDocument && prepared.Blobs[index].DocumentRole == role {
			return &prepared.Blobs[index]
		}
	}
	require.FailNow(t, "the prepared artifact has no such document blob", string(role))
	return nil
}

// TestPrepare_RecordIsIndependentOfTheRequestAndMetadata proves the pointer and byte-slice fields
// the producer is handed (provenance and kind data) are copied into the sealed Record.
func TestPrepare_RecordIsIndependentOfTheRequestAndMetadata(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	provenance := &artifact.Provenance{
		CI:     &artifact.CIProvenance{Provider: "github", RunID: "1"},
		Source: &artifact.SourceProvenance{Commit: "abc"},
	}
	kindData := artifact.KindData(`{"k":1}`)
	req := request(src, artifact.PackagingTar)
	req.Metadata.Provenance = provenance
	source := func(context.Context, string) (produce.ProducerMetadata, error) {
		return produce.ProducerMetadata{KindData: kindData}, nil
	}
	prepared, _ := prepare(t, req, produce.WithMetadataSource(source))
	want, err := prepared.Record.Canonical()
	require.NoError(t, err)

	provenance.CI.RunID = "scribble"
	provenance.Source.Commit = "scribble"
	kindData[0] = 'X'

	got, err := prepared.Record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, want, got, "later edits to the request or the metadata do not reach the Record")
	assert.Equal(t, "1", prepared.Record.Metadata.Provenance.CI.RunID)
	assert.JSONEq(t, `{"k":1}`, string(prepared.Record.KindData))
}
