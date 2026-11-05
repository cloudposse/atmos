package materialize_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
)

var fixedTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

func prepared(t *testing.T, src string, packaging artifact.Packaging) *artifact.PreparedArtifact {
	t.Helper()
	p, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: "pkg/example", Version: "1.0.0", Packaging: packaging, OperationID: "op", PreparedAt: fixedTime,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// contentOf converts staged blobs into the streams a repository would return.
func contentOf(t *testing.T, p *artifact.PreparedArtifact) *artifact.Content {
	t.Helper()
	record := p.Record
	content := &artifact.Content{Record: &record, Representation: artifact.Representation{Packaging: p.Packaging}}
	for index := range p.Blobs {
		blob := &p.Blobs[index]
		reader, err := blob.Open(context.Background())
		require.NoError(t, err)
		content.Streams = append(content.Streams, artifact.ContentStream{Descriptor: blob.Descriptor, Role: blob.Role, Path: blob.Path, Reader: reader})
	}
	t.Cleanup(func() { _ = content.Close() })
	return content
}

func dest(t *testing.T) string {
	return filepath.Join(t.TempDir(), "out", "pulled")
}

func assertNoScratch(t *testing.T, destination string) {
	t.Helper()
	items, err := os.ReadDir(filepath.Dir(destination))
	require.NoError(t, err)
	for _, item := range items {
		assert.NotContains(t, item.Name(), ".atmos-pull-", "staging must be removed")
	}
}

func TestMaterialize_RoundTripEveryPackaging(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			src := t.TempDir()
			pkg := fixtures.WriteFilePackage(t, src)
			p := prepared(t, src, packaging)
			out := dest(t)
			require.NoError(t, materialize.Materialize(context.Background(), contentOf(t, p), out))
			assertNoScratch(t, out)

			state, err := materialize.InspectDestination(out, &p.Record)
			require.NoError(t, err)
			assert.Equal(t, materialize.DestinationMatches, state)

			data, err := os.ReadFile(filepath.Join(out, "bin", "helper.sh"))
			require.NoError(t, err)
			assert.Equal(t, fixtures.HelperScript, string(data))
			if runtime.GOOS != "windows" {
				info, err := os.Stat(filepath.Join(out, "bin", "helper.sh"))
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "executable bit survives")
			}
			_, err = os.Stat(filepath.Join(out, "emptydir"))
			require.NoError(t, err)
			_, err = os.Stat(filepath.Join(out, fixtures.UnicodeName))
			require.NoError(t, err)
			_, err = os.Lstat(filepath.Join(out, ".atmos"))
			assert.True(t, os.IsNotExist(err), "managed distribution members are not materialized")
			if pkg.Symlinks {
				target, err := os.Readlink(filepath.Join(out, "link-to-file"))
				require.NoError(t, err)
				assert.Equal(t, "nested/deep/file.txt", filepath.ToSlash(target))
			}
		})
	}
}

func TestMaterialize_ReplacesWithoutMergingAndStaysInsideDestination(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "new.txt", "new", 0o644)
	p := prepared(t, src, artifact.PackagingTgz)

	root := t.TempDir()
	out := filepath.Join(root, "pulled")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(outside, 0o755))
	fixtures.WriteFile(t, outside, "keep.txt", "keep", 0o644)
	fixtures.WriteFile(t, root, "sibling.txt", "sibling", 0o644)
	fixtures.WriteFile(t, out, "stale.txt", "stale", 0o644)
	fixtures.WriteFile(t, out, "nested/stale2.txt", "stale", 0o644)
	linkOK := fixtures.Symlink(t, out, "link-out", "../outside")

	state, err := materialize.InspectDestination(out, &p.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationDiffers, state)

	require.NoError(t, materialize.Materialize(context.Background(), contentOf(t, p), out))
	items, err := os.ReadDir(out)
	require.NoError(t, err)
	require.Len(t, items, 1, "no stale file is merged with the new tree")
	assert.Equal(t, "new.txt", items[0].Name())
	assertNoScratch(t, out)

	keep, err := os.ReadFile(filepath.Join(outside, "keep.txt"))
	require.NoError(t, err, "a stale link must not let deletion escape the destination")
	assert.Equal(t, "keep", string(keep))
	_, err = os.Stat(filepath.Join(root, "sibling.txt"))
	require.NoError(t, err)
	_ = linkOK
}

func TestInspectDestination_States(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", 0o644)
	p := prepared(t, src, artifact.PackagingNone)
	root := t.TempDir()

	state, err := materialize.InspectDestination(filepath.Join(root, "absent"), &p.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationAbsent, state)

	empty := filepath.Join(root, "empty")
	require.NoError(t, os.Mkdir(empty, 0o755))
	state, err = materialize.InspectDestination(empty, &p.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationEmpty, state)

	differs := filepath.Join(root, "differs")
	fixtures.WriteFile(t, differs, "a.txt", "different", 0o644)
	state, err = materialize.InspectDestination(differs, &p.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationDiffers, state)

	extra := filepath.Join(root, "extra")
	fixtures.WriteFile(t, extra, "a.txt", "a", 0o644)
	fixtures.WriteFile(t, extra, "b.txt", "b", 0o644)
	state, err = materialize.InspectDestination(extra, &p.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationDiffers, state, "extra files are a difference")

	file := filepath.Join(root, "file")
	fixtures.WriteFile(t, root, "file", "x", 0o644)
	_, err = materialize.InspectDestination(file, &p.Record)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)

	if fixtures.Symlink(t, root, "linked", "empty") {
		_, err = materialize.InspectDestination(filepath.Join(root, "linked"), &p.Record)
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
		err = materialize.Materialize(context.Background(), contentOf(t, p), filepath.Join(root, "linked"))
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
	}
}

func TestMaterialize_PerFileIntegrityFailures(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "aaaa", 0o644)
	fixtures.WriteFile(t, src, "b.txt", "bbbb", 0o644)
	tests := []struct {
		name   string
		mutate func(content *artifact.Content)
		want   error
	}{
		{"tampered bytes", func(c *artifact.Content) {
			c.Streams[0].Reader = io.NopCloser(bytes.NewReader([]byte("xxxx")))
		}, errUtils.ErrArtifactIntegrityFailed},
		{"truncated", func(c *artifact.Content) {
			c.Streams[0].Reader = io.NopCloser(bytes.NewReader([]byte("aa")))
		}, errUtils.ErrArtifactIntegrityFailed},
		{"oversize", func(c *artifact.Content) {
			c.Streams[0].Reader = io.NopCloser(bytes.NewReader([]byte("aaaaaaaa")))
		}, errUtils.ErrArtifactTooLarge},
		{"missing payload", func(c *artifact.Content) { c.Streams = c.Streams[1:] }, errUtils.ErrArtifactIntegrityFailed},
		{"extra payload", func(c *artifact.Content) {
			c.Streams = append(c.Streams, artifact.ContentStream{Role: artifact.BlobPayload, Path: "extra.txt", Reader: io.NopCloser(bytes.NewReader(nil))})
		}, errUtils.ErrArtifactIntegrityFailed},
		{"duplicate payload", func(c *artifact.Content) {
			c.Streams = append(c.Streams, c.Streams[0])
		}, errUtils.ErrArtifactIntegrityFailed},
		{"traversal path", func(c *artifact.Content) {
			c.Streams[0].Path = "../a.txt"
		}, errUtils.ErrArtifactIntegrityFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := contentOf(t, prepared(t, src, artifact.PackagingNone))
			tt.mutate(content)
			out := dest(t)
			err := materialize.Materialize(context.Background(), content, out)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			_, statErr := os.Lstat(out)
			assert.True(t, os.IsNotExist(statErr), "a failed extraction never creates the destination")
			assertNoScratch(t, out)
		})
	}
}

func TestMaterialize_GraphWithoutRecordIsUnsupported(t *testing.T) {
	err := materialize.Materialize(context.Background(), &artifact.Content{}, dest(t))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	err = materialize.Materialize(context.Background(), nil, dest(t))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	require.ErrorIs(t, materialize.Materialize(context.Background(), &artifact.Content{}, ""), errUtils.ErrArtifactConfigInvalid)
}

func TestMaterialize_SizeLimit(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "0123456789", 0o644)
	content := contentOf(t, prepared(t, src, artifact.PackagingTgz))
	err := materialize.Materialize(context.Background(), content, dest(t), materialize.WithMaxBytes(5))
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}

func TestMaterialize_CanceledContextCleansUp(t *testing.T) {
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)
	for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTgz, artifact.PackagingZip} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		out := dest(t)
		err := materialize.Materialize(ctx, contentOf(t, prepared(t, src, packaging)), out)
		require.ErrorIs(t, err, context.Canceled, packaging)
		_, statErr := os.Lstat(out)
		assert.True(t, os.IsNotExist(statErr))
		assertNoScratch(t, out)
	}
}

func TestMaterialize_RestrictiveDirectoryModeAppliedLast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permission bits are not enforced on Windows")
	}
	src := t.TempDir()
	fixtures.WriteFile(t, src, "locked/inner.txt", "x", 0o644)
	require.NoError(t, os.Chmod(filepath.Join(src, "locked"), 0o555))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(src, "locked"), 0o755) })
	p := prepared(t, src, artifact.PackagingTar)
	out := dest(t)
	require.NoError(t, materialize.Materialize(context.Background(), contentOf(t, p), out))
	info, err := os.Stat(filepath.Join(out, "locked"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o555), info.Mode().Perm())
	require.NoError(t, os.Chmod(filepath.Join(out, "locked"), 0o755))
}

// sealedRecord builds a valid Record around an inventory for crafted-content tests.
func sealedRecord(t *testing.T, inventory []artifact.Entry) artifact.Record {
	t.Helper()
	record := artifact.Record{
		Name: "pkg/crafted", Version: "1", Kind: artifact.KindFiles,
		CreatedAt: artifact.NewTimestamp(fixedTime), Inventory: inventory,
	}
	require.NoError(t, record.Seal())
	return record
}

func fileEntry(path, content string, mode uint32) artifact.Entry {
	return artifact.Entry{Path: path, Type: artifact.EntryFile, Size: int64(len(content)), Mode: mode, Digest: digest.FromString(content)}
}

type tarMember struct {
	header tar.Header
	body   string
}

func tarOf(t *testing.T, members []tarMember) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for index := range members {
		m := &members[index]
		h := m.header
		h.Size = int64(len(m.body))
		require.NoError(t, tw.WriteHeader(&h))
		_, err := tw.Write([]byte(m.body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	return io.NopCloser(&buf)
}

func tgzOf(t *testing.T, members []tarMember) io.ReadCloser {
	t.Helper()
	raw, err := io.ReadAll(tarOf(t, members))
	require.NoError(t, err)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err = gz.Write(raw)
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return io.NopCloser(&buf)
}

func archiveContent(record *artifact.Record, packaging artifact.Packaging, reader io.ReadCloser) *artifact.Content {
	return &artifact.Content{
		Record:         record,
		Representation: artifact.Representation{Packaging: packaging},
		Streams:        []artifact.ContentStream{{Role: artifact.BlobPayload, Reader: reader}},
	}
}

func TestMaterialize_HostileArchives(t *testing.T) {
	record := sealedRecord(t, []artifact.Entry{fileEntry("good.txt", "good", 0o644)})
	good := tarMember{header: tar.Header{Name: "good.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "good"}
	tests := []struct {
		name    string
		members []tarMember
		want    error
	}{
		{"traversal", []tarMember{{header: tar.Header{Name: "../evil.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "x"}, good}, errUtils.ErrArtifactUnsafePath},
		{"nested traversal", []tarMember{{header: tar.Header{Name: "a/../../evil.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "x"}, good}, errUtils.ErrArtifactUnsafePath},
		{"absolute", []tarMember{{header: tar.Header{Name: "/etc/passwd", Typeflag: tar.TypeReg, Mode: 0o644}, body: "x"}, good}, errUtils.ErrArtifactUnsafePath},
		{"not in inventory", []tarMember{good, {header: tar.Header{Name: "extra.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "x"}}, errUtils.ErrArtifactIntegrityFailed},
		{"hard link", []tarMember{{header: tar.Header{Name: "good.txt", Typeflag: tar.TypeLink, Linkname: "other"}}}, errUtils.ErrArtifactPackagingInvalid},
		{"device", []tarMember{{header: tar.Header{Name: "dev", Typeflag: tar.TypeChar}}}, errUtils.ErrArtifactPackagingInvalid},
		{"wrong size", []tarMember{{header: tar.Header{Name: "good.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "goodgood"}}, errUtils.ErrArtifactIntegrityFailed},
		{"wrong digest", []tarMember{{header: tar.Header{Name: "good.txt", Typeflag: tar.TypeReg, Mode: 0o644}, body: "evil"}}, errUtils.ErrArtifactIntegrityFailed},
		{"duplicate", []tarMember{good, good}, errUtils.ErrArtifactIntegrityFailed},
		{"missing member", nil, errUtils.ErrArtifactIntegrityFailed},
		{"unexpected managed member", []tarMember{good, {header: tar.Header{Name: ".atmos/artifact/evil.sh", Typeflag: tar.TypeReg, Mode: 0o755}, body: "x"}}, errUtils.ErrArtifactPackagingInvalid},
		{"tampered manifest", []tarMember{good, {header: tar.Header{Name: ".atmos/artifact/manifest.json", Typeflag: tar.TypeReg, Mode: 0o644}, body: "{}"}}, errUtils.ErrArtifactIntegrityFailed},
	}
	for _, tt := range tests {
		for _, wrap := range []struct {
			name      string
			packaging artifact.Packaging
			build     func(*testing.T, []tarMember) io.ReadCloser
		}{{"tar", artifact.PackagingTar, tarOf}, {"tgz", artifact.PackagingTgz, tgzOf}} {
			t.Run(tt.name+"/"+wrap.name, func(t *testing.T) {
				out := dest(t)
				err := materialize.Materialize(context.Background(), archiveContent(&record, wrap.packaging, wrap.build(t, tt.members)), out)
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.want)
				_, statErr := os.Lstat(out)
				assert.True(t, os.IsNotExist(statErr))
				assertNoScratch(t, out)
			})
		}
	}
}

func TestMaterialize_ArchiveRejectsCorruptStreams(t *testing.T) {
	record := sealedRecord(t, []artifact.Entry{fileEntry("good.txt", "good", 0o644)})
	for _, packaging := range []artifact.Packaging{artifact.PackagingTgz, artifact.PackagingZip, artifact.PackagingTar} {
		err := materialize.Materialize(context.Background(), archiveContent(&record, packaging, io.NopCloser(bytes.NewReader([]byte("not an archive")))), dest(t))
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, packaging)
	}
	noPayload := &artifact.Content{Record: &record, Representation: artifact.Representation{Packaging: artifact.PackagingTar}}
	require.ErrorIs(t, materialize.Materialize(context.Background(), noPayload, dest(t)), errUtils.ErrArtifactIntegrityFailed)
	badPackaging := archiveContent(&record, "rar", io.NopCloser(bytes.NewReader(nil)))
	require.ErrorIs(t, materialize.Materialize(context.Background(), badPackaging, dest(t)), errUtils.ErrArtifactPackagingInvalid)
}

func TestMaterialize_ChainedSymlinkEscapeIsRejected(t *testing.T) {
	record := sealedRecord(t, []artifact.Entry{
		{Path: "a", Type: artifact.EntryDirectory, Mode: 0o755},
		{Path: "a/b", Type: artifact.EntrySymlink, Target: ".."},
		{Path: "c", Type: artifact.EntrySymlink, Target: "a/b/../.."},
	})
	content := &artifact.Content{Record: &record, Representation: artifact.Representation{Packaging: artifact.PackagingNone}}
	out := dest(t)
	err := materialize.Materialize(context.Background(), content, out)
	if err == nil {
		t.Skip("this platform cannot create the symlinks needed for the chained escape")
	}
	if !assert.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath) {
		t.Logf("unexpected error: %v", err)
	}
	_, statErr := os.Lstat(out)
	assert.True(t, os.IsNotExist(statErr))
}
