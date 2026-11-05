package produce_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
)

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func request(src string, packaging artifact.Packaging) *artifact.PrepareRequest {
	return &artifact.PrepareRequest{
		Source:      src,
		Name:        "tools/example",
		Version:     "1.0.0",
		Packaging:   packaging,
		OperationID: "op-1",
		PreparedAt:  fixedTime,
	}
}

func prepare(t *testing.T, req *artifact.PrepareRequest, opts ...produce.Option) (*artifact.PreparedArtifact, string) {
	t.Helper()
	staging := t.TempDir()
	opts = append([]produce.Option{produce.WithStagingRoot(staging)}, opts...)
	prepared, err := produce.New(opts...).Prepare(context.Background(), *req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	return prepared, staging
}

func readBlob(t *testing.T, blob *artifact.StagedBlob) []byte {
	t.Helper()
	reader, err := blob.Open(context.Background())
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return data
}

func payloadBlobs(prepared *artifact.PreparedArtifact) []artifact.StagedBlob {
	var blobs []artifact.StagedBlob
	for index := range prepared.Blobs {
		if prepared.Blobs[index].Role == artifact.BlobPayload {
			blobs = append(blobs, prepared.Blobs[index])
		}
	}
	return blobs
}

func inventoryPaths(record *artifact.Record) []string {
	paths := make([]string, 0, len(record.Inventory))
	for _, entry := range record.Inventory {
		paths = append(paths, entry.Path)
	}
	return paths
}

func TestFilesProducer_FilePackageRecord(t *testing.T) {
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	prepared, _ := prepare(t, request(src, artifact.PackagingNone))
	record := prepared.Record

	assert.Equal(t, fixtures.FilePackagePaths(pkg.Symlinks), inventoryPaths(&record), "reserved directory is not payload; root README is")
	assert.Equal(t, artifact.KindFiles, record.Kind)
	assert.Equal(t, "tools/example", record.Name)
	assert.Equal(t, artifact.NewTimestamp(fixedTime), record.CreatedAt)
	require.Len(t, record.Documents, 2)
	assert.Equal(t, artifact.DocumentChangelog, record.Documents[0].Role)
	assert.Equal(t, ".atmos/artifact/CHANGELOG.md", record.Documents[0].SourcePath)
	assert.Equal(t, artifact.DocumentReadme, record.Documents[1].Role)
	assert.Equal(t, int64(len(fixtures.PackageReadme)), record.Documents[1].Size)
	assert.Equal(t, "op-1", prepared.OperationID)
	assert.True(t, fixedTime.Equal(prepared.PreparedAt))
	require.NoError(t, record.Validate())

	for _, entry := range record.Inventory {
		switch entry.Path {
		case "bin/helper.sh":
			if fixtures.POSIXModes() {
				assert.Equal(t, uint32(0o755), entry.Mode)
			}
		case "empty.txt":
			assert.Zero(t, entry.Size)
		}
	}
}

func TestFilesProducer_PerFileBlobs(t *testing.T) {
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	prepared, _ := prepare(t, request(src, artifact.PackagingNone))

	var files []string
	blobs := payloadBlobs(prepared)
	for index := range blobs {
		files = append(files, blobs[index].Path)
		assert.NotEmpty(t, blobs[index].Path)
		assert.Equal(t, int64(len(readBlob(t, &blobs[index]))), blobs[index].Descriptor.Size)
	}
	want := []string{"README.md", "bin/helper.sh", "empty.txt", "nested/deep/file.txt", fixtures.UnicodeName}
	assert.ElementsMatch(t, want, files, "directories and symlinks have no blob (symlinks=%v)", pkg.Symlinks)

	documents := 0
	for _, blob := range prepared.Blobs {
		if blob.Role == artifact.BlobDocument {
			documents++
			assert.Equal(t, artifact.DocumentMediaType, blob.Descriptor.MediaType)
		}
	}
	assert.Equal(t, 2, documents)
}

func TestFilesProducer_RepeatedPreparationIsByteIdentical(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFilePackage(t, src)
			first, _ := prepare(t, request(src, packaging))
			// Touch mtimes so only content, paths, and modes can influence the output.
			future := time.Now().Add(48 * time.Hour)
			require.NoError(t, os.Chtimes(filepath.Join(src, "README.md"), future, future))
			second, _ := prepare(t, request(src, packaging))

			firstRecord, err := first.Record.Canonical()
			require.NoError(t, err)
			secondRecord, err := second.Record.Canonical()
			require.NoError(t, err)
			assert.Equal(t, firstRecord, secondRecord)
			assert.Equal(t, first.IntentDigest, second.IntentDigest)
			require.Len(t, second.Blobs, len(first.Blobs))
			for index := range first.Blobs {
				assert.Equal(t, first.Blobs[index].Descriptor, second.Blobs[index].Descriptor)
				assert.Equal(t, readBlob(t, &first.Blobs[index]), readBlob(t, &second.Blobs[index]))
			}
		})
	}
}

func TestFilesProducer_IntentIgnoresTimeButRecordDoesNot(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	first, _ := prepare(t, request(src, artifact.PackagingTgz))
	later := request(src, artifact.PackagingTgz)
	later.PreparedAt = fixedTime.Add(26 * time.Hour)
	later.OperationID = "op-2"
	second, _ := prepare(t, later)

	assert.Equal(t, first.IntentDigest, second.IntentDigest)
	assert.Equal(t, first.Record.ContentDigest, second.Record.ContentDigest)
	firstDigest, err := first.Record.RecordDigest()
	require.NoError(t, err)
	secondDigest, err := second.Record.RecordDigest()
	require.NoError(t, err)
	assert.NotEqual(t, firstDigest, secondDigest)
}

func TestFilesProducer_ChangedModeLabelOrDocumentChangesIntent(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	base, _ := prepare(t, request(src, artifact.PackagingNone))

	labeled := request(src, artifact.PackagingNone)
	labeled.Metadata.Labels = map[string]string{"team": "platform"}
	withLabel, _ := prepare(t, labeled)
	assert.NotEqual(t, base.IntentDigest, withLabel.IntentDigest)
	assert.Equal(t, base.Record.ContentDigest, withLabel.Record.ContentDigest, "labels do not change content identity")

	withMode := base
	if fixtures.POSIXModes() {
		require.NoError(t, os.Chmod(filepath.Join(src, "nested", "deep", "file.txt"), 0o755))
		withMode, _ = prepare(t, request(src, artifact.PackagingNone))
		assert.NotEqual(t, base.Record.ContentDigest, withMode.Record.ContentDigest)
	}

	fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "changed readme\n", 0o644)
	withDoc, _ := prepare(t, request(src, artifact.PackagingNone))
	assert.NotEqual(t, withMode.Record.ContentDigest, withDoc.Record.ContentDigest)
}

func TestFilesProducer_ArchiveCarriesManifestAndLeavesSourceUntouched(t *testing.T) {
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	before := listTree(t, src)
	prepared, _ := prepare(t, request(src, artifact.PackagingTgz))
	assert.Equal(t, before, listTree(t, src), "the source tree must not be modified")

	blobs := payloadBlobs(prepared)
	require.Len(t, blobs, 1)
	assert.Equal(t, artifact.MediaTypeTarGzip, blobs[0].Descriptor.MediaType)
	data := readBlob(t, &blobs[0])
	assert.Equal(t, int64(len(data)), blobs[0].Descriptor.Size)

	gz, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	reader := tar.NewReader(gz)
	contents := map[string][]byte{}
	modes := map[string]int64{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		modes[header.Name] = header.Mode
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		contents[header.Name] = body
	}
	recordBytes, err := prepared.Record.Canonical()
	require.NoError(t, err)
	assert.Equal(t, recordBytes, contents[".atmos/artifact/manifest.json"])
	assert.Equal(t, fixtures.PackageReadme, string(contents[".atmos/artifact/README.md"]))
	assert.Equal(t, fixtures.PackageChangelog, string(contents[".atmos/artifact/CHANGELOG.md"]))
	assert.Equal(t, fixtures.RootReadme, string(contents["README.md"]), "root README stays ordinary payload")
	if fixtures.POSIXModes() {
		assert.Equal(t, int64(0o755), modes["bin/helper.sh"])
	}
	assert.Contains(t, contents, "emptydir/")
	if pkg.Symlinks {
		assert.Contains(t, modes, "link-to-file")
	}

	for _, entry := range prepared.Record.Inventory {
		assert.NotContains(t, entry.Path, "manifest.json", "generated members never enter the Record")
		assert.NotContains(t, entry.Path, ".atmos/artifact")
	}
}

func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		require.NoError(t, err)
		out = append(out, path+"|"+info.Mode().String()+"|"+info.ModTime().String())
		return nil
	}))
	return out
}

func TestFilesProducer_IdentityPrecedence(t *testing.T) {
	src := filepath.Join(t.TempDir(), "my-source")
	require.NoError(t, os.Mkdir(src, 0o755))
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	metadata := func(context.Context, string) (produce.ProducerMetadata, error) {
		return produce.ProducerMetadata{Name: "from/producer", Version: "9.9.9", Description: "producer desc", Labels: map[string]string{"a": "producer", "b": "producer"}}, nil
	}
	tests := []struct {
		name         string
		explicitName string
		explicitVer  string
		opts         []produce.Option
		wantName     string
		wantVersion  string
		wantErr      error
	}{
		{"explicit beats producer", "explicit/name", "2.0.0", []produce.Option{produce.WithMetadataSource(metadata)}, "explicit/name", "2.0.0", nil},
		{"producer beats basename", "", "", []produce.Option{produce.WithMetadataSource(metadata)}, "from/producer", "9.9.9", nil},
		{"basename fallback", "", "1.0.0", nil, "my-source", "1.0.0", nil},
		{"version never generated", "x", "", nil, "", "", errUtils.ErrArtifactRefInvalid},
		{"invalid explicit name", "Bad Name", "1", nil, "", "", errUtils.ErrArtifactRefInvalid},
		{"invalid version", "x", "has space", nil, "", "", errUtils.ErrArtifactRefInvalid},
		{"traversal name", "../x", "1", nil, "", "", errUtils.ErrArtifactUnsafePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request(src, artifact.PackagingNone)
			req.Name, req.Version = tt.explicitName, tt.explicitVer
			staging := t.TempDir()
			prepared, err := produce.New(append([]produce.Option{produce.WithStagingRoot(staging)}, tt.opts...)...).Prepare(context.Background(), *req)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				items, readErr := os.ReadDir(staging)
				require.NoError(t, readErr)
				assert.Empty(t, items, "nothing is staged for an invalid identity")
				return
			}
			require.NoError(t, err)
			defer prepared.Close()
			assert.Equal(t, tt.wantName, prepared.Record.Name)
			assert.Equal(t, tt.wantVersion, prepared.Record.Version)
		})
	}
}

func TestFilesProducer_InvalidBasenameNeedsExplicitName(t *testing.T) {
	src := filepath.Join(t.TempDir(), "My Source")
	require.NoError(t, os.Mkdir(src, 0o755))
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	req := request(src, artifact.PackagingNone)
	req.Name = ""
	_, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	assert.Contains(t, err.Error(), "explicit name")
}

func TestFilesProducer_LabelMergeAndDescription(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	metadata := func(context.Context, string) (produce.ProducerMetadata, error) {
		return produce.ProducerMetadata{Description: "producer desc", Labels: map[string]string{"a": "producer", "b": "producer"}}, nil
	}
	req := request(src, artifact.PackagingNone)
	req.Metadata = artifact.Metadata{Labels: map[string]string{"b": "explicit", "c": "explicit"}}
	prepared, _ := prepare(t, req, produce.WithMetadataSource(metadata))
	assert.Equal(t, map[string]string{"a": "producer", "b": "explicit", "c": "explicit"}, prepared.Record.Metadata.Labels)
	assert.Equal(t, "producer desc", prepared.Record.Metadata.Description)

	req.Metadata.Description = "explicit desc"
	prepared, _ = prepare(t, req, produce.WithMetadataSource(metadata))
	assert.Equal(t, "explicit desc", prepared.Record.Metadata.Description)
}

func TestFilesProducer_InvalidLabelsRejectedBeforeStaging(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	req := request(src, artifact.PackagingNone)
	req.Metadata.Labels = map[string]string{"bad=key": "v"}
	staging := t.TempDir()
	_, err := produce.New(produce.WithStagingRoot(staging)).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	items, readErr := os.ReadDir(staging)
	require.NoError(t, readErr)
	assert.Empty(t, items)
}

func TestFilesProducer_RequestValidation(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	base := request(src, artifact.PackagingNone)
	tests := []struct {
		name   string
		mutate func(*artifact.PrepareRequest)
		want   error
	}{
		{"other kind", func(r *artifact.PrepareRequest) { r.Kind = "scaffold" }, errUtils.ErrArtifactConfigInvalid},
		{"no operation id", func(r *artifact.PrepareRequest) { r.OperationID = "" }, errUtils.ErrArtifactConfigInvalid},
		{"no prepared time", func(r *artifact.PrepareRequest) { r.PreparedAt = time.Time{} }, errUtils.ErrArtifactConfigInvalid},
		{"bad packaging", func(r *artifact.PrepareRequest) { r.Packaging = "rar" }, errUtils.ErrArtifactPackagingInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := *base
			tt.mutate(&req)
			_, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), req)
			require.ErrorIs(t, err, tt.want)
		})
	}
	assert.Equal(t, artifact.KindFiles, produce.New().Kind())
}

func TestFilesProducer_ReservedDirectoryConflicts(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, dir string) bool
	}{
		{"source manifest.json", func(t *testing.T, dir string) bool {
			fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/manifest.json", "{}", 0o644)
			return true
		}},
		{"unknown reserved file", func(t *testing.T, dir string) bool {
			fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/notes.txt", "x", 0o644)
			return true
		}},
		{"nested reserved directory", func(t *testing.T, dir string) bool {
			fixtures.WriteFile(t, dir, fixtures.ReservedDir+"/sub/README.md", "x", 0o644)
			return true
		}},
		{"reserved symlink document", func(t *testing.T, dir string) bool {
			fixtures.WriteFile(t, dir, "real.md", "x", 0o644)
			return fixtures.Symlink(t, dir, fixtures.ReservedDir+"/README.md", "../../real.md")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
			if !tt.build(t, src) {
				t.Skip("fixture unavailable")
			}
			staging := t.TempDir()
			_, err := produce.New(produce.WithStagingRoot(staging)).Prepare(context.Background(), *request(src, artifact.PackagingTgz))
			require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
			items, readErr := os.ReadDir(staging)
			require.NoError(t, readErr)
			assert.Empty(t, items)
		})
	}
}

func TestFilesProducer_OtherAtmosContentStaysPayload(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, ".atmos/other/config.yaml", "x: 1\n", 0o644)
	fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "doc", 0o644)
	prepared, _ := prepare(t, request(src, artifact.PackagingTar))
	assert.Equal(t, []string{".atmos", ".atmos/other", ".atmos/other/config.yaml"}, inventoryPaths(&prepared.Record))
}

func TestFilesProducer_ReservedParentDroppedWhenOnlyDistribution(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "doc", 0o644)
	prepared, _ := prepare(t, request(src, artifact.PackagingNone))
	assert.Equal(t, []string{"a.txt"}, inventoryPaths(&prepared.Record))
}

func TestFilesProducer_ExplicitDocuments(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, src, "docs/GUIDE.md", "explicit guide", 0o644)
	fixtures.WriteFile(t, src, "docs/HISTORY.md", "explicit history", 0o644)
	fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "reserved readme", 0o644)

	req := request(src, artifact.PackagingNone)
	req.Readme = "docs/GUIDE.md"
	req.Changelog = "docs/HISTORY.md"
	prepared, _ := prepare(t, req)
	byRole := map[artifact.DocumentRole]artifact.Document{}
	for _, doc := range prepared.Record.Documents {
		byRole[doc.Role] = doc
	}
	assert.Equal(t, "docs/GUIDE.md", byRole[artifact.DocumentReadme].SourcePath, "explicit override wins over the reserved file")
	assert.Equal(t, "docs/HISTORY.md", byRole[artifact.DocumentChangelog].SourcePath)
	assert.Contains(t, inventoryPaths(&prepared.Record), "docs/GUIDE.md", "an explicit document inside the source stays payload")
}

func TestFilesProducer_ExplicitDocumentRejections(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, src, "big.md", string(bytes.Repeat([]byte("x"), int(artifact.MaxDocumentSize)+1)), 0o644)
	fixtures.MkdirAll(t, src, "adir")
	tests := []struct {
		name     string
		override string
		want     error
	}{
		{"traversal", "../outside.md", errUtils.ErrArtifactUnsafePath},
		{"absolute", filepath.Join(src, "a.txt"), errUtils.ErrArtifactUnsafePath},
		{"dot", ".", errUtils.ErrArtifactUnsafePath},
		{"missing", "missing.md", errUtils.ErrArtifactPackagingInvalid},
		{"directory", "adir", errUtils.ErrArtifactPackagingInvalid},
		{"oversize", "big.md", errUtils.ErrArtifactTooLarge},
		{"reserved namespace", artifact.ReservedNamespace + "/x.md", errUtils.ErrArtifactUnsafePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := request(src, artifact.PackagingNone)
			req.Readme = tt.override
			_, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestFilesProducer_SingleFileSource(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "tool.bin", "binary", 0o755)
	fixtures.WriteFile(t, dir, "NOTES.md", "sibling notes", 0o644)
	fixtures.MkdirAll(t, dir, "sub")
	req := request(filepath.Join(dir, "tool.bin"), artifact.PackagingZip)
	req.Name = ""
	req.Readme = "NOTES.md"
	prepared, _ := prepare(t, req)
	assert.Equal(t, "tool.bin", prepared.Record.Name)
	assert.Equal(t, []string{"tool.bin"}, inventoryPaths(&prepared.Record))
	require.Len(t, prepared.Record.Documents, 1)
	assert.Equal(t, "NOTES.md", prepared.Record.Documents[0].SourcePath)

	req.Readme = "../outside.md"
	_, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)

	req.Readme = "sub"
	_, err = produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)

	req.Readme = "missing.md"
	_, err = produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
}

func TestFilesProducer_SingleFileDocumentEscapingThroughSymlink(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(parent, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	fixtures.WriteFile(t, parent, "tool.bin", "x", 0o644)
	fixtures.WriteFile(t, outside, "secret.md", "secret", 0o644)
	if err := os.Symlink(outside, filepath.Join(parent, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	req := request(filepath.Join(parent, "tool.bin"), artifact.PackagingNone)
	req.Readme = "link/secret.md"
	_, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), *req)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
}

func TestFilesProducer_InvalidPackagesFailBeforeAnythingIsKept(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T, dir string) bool
		want  error
	}{
		{"symlink escape", func(t *testing.T, dir string) bool { return fixtures.Symlink(t, dir, "escape", "../../etc/passwd") }, errUtils.ErrArtifactUnsafePath},
		{"absolute symlink", func(t *testing.T, dir string) bool {
			return fixtures.Symlink(t, dir, "abs", filepath.ToSlash(os.TempDir()))
		}, errUtils.ErrArtifactUnsafePath},
		{"special file", func(t *testing.T, dir string) bool { return fixtures.SpecialFile(t, dir, "pipe") }, errUtils.ErrArtifactPackagingInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := t.TempDir()
			fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
			if !tt.build(t, src) {
				t.Skip("fixture unavailable")
			}
			staging := t.TempDir()
			_, err := produce.New(produce.WithStagingRoot(staging)).Prepare(context.Background(), *request(src, artifact.PackagingNone))
			require.ErrorIs(t, err, tt.want)
			items, readErr := os.ReadDir(staging)
			require.NoError(t, readErr)
			assert.Empty(t, items)
		})
	}
}

func TestFilesProducer_ChangeDuringCaptureFails(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "aaaa", 0o644)
	fixtures.WriteFile(t, src, "b.txt", "bbbb", 0o644)
	staging := t.TempDir()
	hook := snapshot.WithCaptureHook(func(path string) {
		if path == "b.txt" {
			fixtures.WriteFile(t, src, "a.txt", "AAAAA", 0o644)
		}
	})
	_, err := produce.New(produce.WithStagingRoot(staging), produce.WithSnapshotOptions(hook)).Prepare(context.Background(), *request(src, artifact.PackagingTgz))
	require.ErrorIs(t, err, errUtils.ErrArtifactSourceChanged)
	items, readErr := os.ReadDir(staging)
	require.NoError(t, readErr)
	assert.Empty(t, items)
}

func TestFilesProducer_CloseRemovesStagingAndIsIdempotent(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	prepared, staging := prepare(t, request(src, artifact.PackagingTgz))
	items, err := os.ReadDir(staging)
	require.NoError(t, err)
	require.NotEmpty(t, items)
	require.NoError(t, prepared.Close())
	require.NoError(t, prepared.Close())
	items, err = os.ReadDir(staging)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestFilesProducer_CancellationRemovesStaging(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, src, "b.txt", "b", 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	staging := t.TempDir()
	hook := snapshot.WithCaptureHook(func(string) { cancel() })
	_, err := produce.New(produce.WithStagingRoot(staging), produce.WithSnapshotOptions(hook)).Prepare(ctx, *request(src, artifact.PackagingTgz))
	require.ErrorIs(t, err, context.Canceled)
	items, readErr := os.ReadDir(staging)
	require.NoError(t, readErr)
	assert.Empty(t, items)
}

func TestFilesProducer_LimitsAreExplicitErrors(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "0123456789", 0o644)
	_, err := produce.New(produce.WithStagingRoot(t.TempDir()), produce.WithLimits(snapshot.Limits{MaxBytes: 5})).Prepare(context.Background(), *request(src, artifact.PackagingNone))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}

func TestRegisterAddsFilesProducer(t *testing.T) {
	registry := artifact.NewProducerRegistry()
	require.NoError(t, produce.Register(registry))
	producer, ok := registry.Lookup("")
	require.True(t, ok)
	assert.Equal(t, artifact.KindFiles, producer.Kind())
	require.ErrorIs(t, produce.Register(registry), errUtils.ErrArtifactConflict)
	assert.Equal(t, []string{artifact.KindFiles}, registry.Kinds())
}
