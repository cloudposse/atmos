package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func fileMember(path, content string, mode fs.FileMode) Member {
	return Member{
		Path: path, Kind: MemberFile, Mode: mode, Size: int64(len(content)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil },
	}
}

func sampleMembers() []Member {
	return []Member{
		fileMember("z/last.txt", "last", 0o644),
		{Path: "z", Kind: MemberDirectory, Mode: 0o755},
		fileMember("bin/run.sh", "#!/bin/sh\n", 0o755),
		{Path: "bin", Kind: MemberDirectory, Mode: 0o755},
		{Path: "link", Kind: MemberSymlink, Target: "z/last.txt"},
		fileMember("unicode-é.txt", "ü", 0o640),
		fileMember("empty", "", 0o644),
	}
}

func TestWriteDeterministic_IdenticalBytesRegardlessOfInputOrder(t *testing.T) {
	for _, format := range []string{FormatTar, FormatTGZ, FormatZip} {
		t.Run(format, func(t *testing.T) {
			forward := sampleMembers()
			reversed := sampleMembers()
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}
			var first, second, third bytes.Buffer
			require.NoError(t, WriteDeterministic(&first, format, forward))
			require.NoError(t, WriteDeterministic(&second, format, reversed))
			require.NoError(t, WriteDeterministic(&third, format, forward))
			assert.Equal(t, first.Bytes(), second.Bytes(), "member order must not change the bytes")
			assert.Equal(t, first.Bytes(), third.Bytes(), "repeated writes must be identical")
		})
	}
}

func TestWriteDeterministic_TarNormalizesMetadata(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteDeterministic(&buf, FormatTar, sampleMembers()))
	reader := tar.NewReader(&buf)
	var names []string
	modes := map[string]int64{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		names = append(names, header.Name)
		modes[header.Name] = header.Mode
		assert.True(t, header.ModTime.Equal(tarEpoch), "%s mtime", header.Name)
		assert.Zero(t, header.Uid)
		assert.Zero(t, header.Gid)
		assert.Empty(t, header.Uname)
		assert.Empty(t, header.Gname)
	}
	assert.Equal(t, []string{"bin/", "bin/run.sh", "empty", "link", "unicode-é.txt", "z/", "z/last.txt"}, names)
	assert.Equal(t, int64(0o755), modes["bin/run.sh"], "executable bit must survive")
	assert.Equal(t, int64(0o640), modes["unicode-é.txt"])
}

func TestWriteDeterministic_TGZHeaderCarriesNoNameOrTime(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteDeterministic(&buf, FormatTGZ, sampleMembers()))
	gz, err := gzip.NewReader(&buf)
	require.NoError(t, err)
	assert.Empty(t, gz.Name)
	assert.True(t, gz.ModTime.IsZero() || gz.ModTime.Unix() == 0)
}

func TestWriteDeterministic_ZipEntries(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, WriteDeterministic(&buf, FormatZip, sampleMembers()))
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	byName := map[string]*zip.File{}
	for _, f := range reader.File {
		byName[f.Name] = f
		assert.True(t, f.Modified.UTC().Equal(zipEpoch), "%s modified", f.Name)
	}
	require.Contains(t, byName, "bin/run.sh")
	assert.Equal(t, fs.FileMode(0o755), byName["bin/run.sh"].Mode().Perm())
	assert.True(t, byName["bin/"].Mode().IsDir())
	assert.NotZero(t, byName["link"].Mode()&fs.ModeSymlink)
	rc, err := byName["link"].Open()
	require.NoError(t, err)
	target, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "z/last.txt", string(target))
}

func TestWriteDeterministic_Rejections(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		members []Member
		want    error
	}{
		{"duplicate path", FormatTar, []Member{fileMember("a", "x", 0o644), fileMember("a", "y", 0o644)}, errUtils.ErrArchiveWriteFailed},
		{"absolute path", FormatTar, []Member{fileMember("/a", "x", 0o644)}, errUtils.ErrArchiveInvalidSubpath},
		{"empty path", FormatZip, []Member{fileMember("", "x", 0o644)}, errUtils.ErrArchiveInvalidSubpath},
		{"unsupported format", "tar.xz", nil, errUtils.ErrArchiveFormatNotImplemented},
		{"file without opener", FormatTar, []Member{{Path: "a", Kind: MemberFile, Size: 1}}, errUtils.ErrArchiveWriteFailed},
		{"unknown kind", FormatTar, []Member{{Path: "a", Kind: MemberKind(9)}}, errUtils.ErrArchiveWriteFailed},
		{"shorter than declared", FormatTar, []Member{{
			Path: "a", Kind: MemberFile, Size: 5,
			Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("ab")), nil },
		}}, errUtils.ErrArchiveWriteFailed},
		{"longer than declared", FormatZip, []Member{{
			Path: "a", Kind: MemberFile, Size: 1,
			Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("abc")), nil },
		}}, errUtils.ErrArchiveWriteFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteDeterministic(io.Discard, tt.format, tt.members)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestWriteDeterministic_ClearsPrivilegedBits(t *testing.T) {
	var buf bytes.Buffer
	member := fileMember("a", "x", 0o4755|fs.ModeSetuid)
	require.NoError(t, WriteDeterministic(&buf, FormatTar, []Member{member}))
	header, err := tar.NewReader(&buf).Next()
	require.NoError(t, err)
	assert.Equal(t, int64(0o755), header.Mode)
}

type failingWriter struct{ after int }

func (f *failingWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("disk full")
	}
	f.after -= len(p)
	return len(p), nil
}

func TestWriteDeterministic_PropagatesWriterFailures(t *testing.T) {
	for _, format := range []string{FormatTar, FormatTGZ, FormatZip} {
		t.Run(format, func(t *testing.T) {
			err := WriteDeterministic(&failingWriter{}, format, sampleMembers())
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArchiveWriteFailed)
		})
	}
}
