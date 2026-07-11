package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeFixture creates a small source tree under dir: a top-level file plus
// one nested under a subdirectory, and one file that should be excluded by
// the include/exclude tests.
func writeFixture(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.js"), []byte("exports.handler = 1;"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "util.js"), []byte("module.exports = {};"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "handler.test.js"), []byte("test();"), 0o644))
}

// zipEntries reads back a zip file's entry names -> content for assertions.
func zipEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	r, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer r.Close()

	out := make(map[string]string, len(r.File))
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		out[f.Name] = string(content)
	}
	return out
}

// tarEntries reads back a tar (optionally gzip-wrapped) file's entry names -> content.
func tarEntries(t *testing.T, path string, gz bool) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var r io.Reader = f
	if gz {
		gzr, err := gzip.NewReader(f)
		require.NoError(t, err)
		defer gzr.Close()
		r = gzr
	}

	out := make(map[string]string)
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		content, err := io.ReadAll(tr)
		require.NoError(t, err)
		out[hdr.Name] = string(content)
	}
	return out
}

func TestRun_Replace(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		destSuffix  string
		readEntries func(t *testing.T, path string) map[string]string
	}{
		{"zip", FormatZip, ".zip", zipEntries},
		{"tar", FormatTar, ".tar", func(t *testing.T, p string) map[string]string { return tarEntries(t, p, false) }},
		{"tgz", FormatTGZ, ".tar.gz", func(t *testing.T, p string) map[string]string { return tarEntries(t, p, true) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src")
			writeFixture(t, src)
			dest := filepath.Join(dir, "out"+tt.destSuffix)

			err := Run(ActionReplace, &PackOptions{
				Source:      src,
				Destination: dest,
				Format:      tt.format,
				Exclude:     []string{"**/*.test.js"},
			})
			require.NoError(t, err)

			entries := tt.readEntries(t, dest)
			assert.Equal(t, "exports.handler = 1;", entries["handler.js"])
			assert.Equal(t, "module.exports = {};", entries["nested/util.js"])
			assert.NotContains(t, entries, "handler.test.js")
		})
	}
}

func TestRun_Replace_FormatInferredFromDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "handler.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest}))

	entries := zipEntries(t, dest)
	assert.Contains(t, entries, "handler.js")
}

func TestRun_Replace_Subpath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "handler.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Subpath: "opt/nodejs"}))

	entries := zipEntries(t, dest)
	assert.Contains(t, entries, "opt/nodejs/handler.js")
	assert.Contains(t, entries, "opt/nodejs/nested/util.js")
}

func TestRun_Replace_IncludeOnly(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "handler.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Include: []string{"nested/**"}}))

	entries := zipEntries(t, dest)
	assert.NotContains(t, entries, "handler.js")
	assert.Contains(t, entries, "nested/util.js")
}

func TestRun_Replace_SingleFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "handler.js")
	require.NoError(t, os.WriteFile(src, []byte("x=1"), 0o644))
	dest := filepath.Join(dir, "handler.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest}))

	entries := zipEntries(t, dest)
	assert.Equal(t, "x=1", entries["handler.js"])
}

func TestRun_Replace_ValidationErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		opts    *PackOptions
		wantErr error
	}{
		{
			name:    "missing source",
			opts:    &PackOptions{Destination: filepath.Join(dir, "out.zip")},
			wantErr: errUtils.ErrArchiveSourceRequired,
		},
		{
			name:    "missing destination",
			opts:    &PackOptions{Source: dir},
			wantErr: errUtils.ErrArchiveDestinationRequired,
		},
		{
			name:    "source does not exist",
			opts:    &PackOptions{Source: filepath.Join(dir, "nope"), Destination: filepath.Join(dir, "out.zip")},
			wantErr: errUtils.ErrArchiveSourceNotFound,
		},
		{
			name:    "unknown format",
			opts:    &PackOptions{Source: dir, Destination: filepath.Join(dir, "out.zip"), Format: "rar"},
			wantErr: errUtils.ErrArchiveUnknownFormat,
		},
		{
			name:    "ambiguous extension",
			opts:    &PackOptions{Source: dir, Destination: filepath.Join(dir, "out.bin")},
			wantErr: errUtils.ErrArchiveUnknownFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(ActionReplace, tt.opts)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), "got %v, want %v", err, tt.wantErr)
		})
	}
}

func TestRun_Replace_FormatNotImplemented(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	for _, format := range []string{FormatTarBz2, FormatTarXz} {
		t.Run(format, func(t *testing.T) {
			err := Run(ActionReplace, &PackOptions{
				Source:      src,
				Destination: filepath.Join(dir, "out."+format),
				Format:      format,
			})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArchiveFormatNotImplemented))
		})
	}
}

func TestRun_CreateExtract_NotImplemented(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	opts := &PackOptions{Source: src, Destination: filepath.Join(dir, "out.zip")}

	for _, action := range []Action{ActionCreate, ActionExtract} {
		t.Run(string(action), func(t *testing.T) {
			err := Run(action, opts)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArchiveActionNotImplemented))
		})
	}
}

func TestRun_Update_Zip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")

	// Build the initial archive, excluding nested/ so it's absent from the baseline.
	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"nested/**", "**/*.test.js"}}))
	before := zipEntries(t, dest)
	require.Contains(t, before, "handler.js")
	require.NotContains(t, before, "nested/util.js")

	// Update with a source scoped to nested/ only, adding nested/util.js as a
	// brand-new entry. handler.js — already in the archive but outside this
	// update's source — must survive untouched.
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "util.js"), []byte("module.exports = {v:2};"), 0o644))
	require.NoError(t, Run(ActionUpdate, &PackOptions{Source: filepath.Join(src, "nested"), Destination: dest, Subpath: "nested"}))

	after := zipEntries(t, dest)
	assert.Equal(t, "exports.handler = 1;", after["handler.js"], "untouched entry must survive the update")
	assert.Equal(t, "module.exports = {v:2};", after["nested/util.js"], "new entry must be added")
}

func TestRun_Update_UncompressedTar(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.tar")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))
	before := tarEntries(t, dest, false)
	require.Equal(t, "exports.handler = 1;", before["handler.js"])

	require.NoError(t, os.WriteFile(filepath.Join(src, "handler.js"), []byte("exports.handler = 2;"), 0o644))
	require.NoError(t, Run(ActionUpdate, &PackOptions{Source: filepath.Join(src, "handler.js"), Destination: dest}))

	after := tarEntries(t, dest, false)
	assert.Equal(t, "exports.handler = 2;", after["handler.js"])
	assert.Equal(t, "module.exports = {};", after["nested/util.js"], "untouched entry must survive the update")
}

func TestRun_Update_RejectsCompressedFormats(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	for _, format := range []string{FormatTGZ, FormatTarBz2, FormatTarXz} {
		t.Run(format, func(t *testing.T) {
			err := Run(ActionUpdate, &PackOptions{Source: src, Destination: filepath.Join(dir, "out."+format), Format: format})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArchiveUpdateUnsupportedFormat))
		})
	}
}

func TestRun_Update_CreatesWhenDestinationMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")

	require.NoError(t, Run(ActionUpdate, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))

	entries := zipEntries(t, dest)
	assert.Contains(t, entries, "handler.js")
}
