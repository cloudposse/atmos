package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRun_NilOptions(t *testing.T) {
	for _, action := range []Action{ActionReplace, ActionUpdate} {
		t.Run(string(action), func(t *testing.T) {
			err := Run(action, nil)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArchiveOptionsRequired))
		})
	}
}

func TestRun_Replace_SingleFile_ExcludedByFilter(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "handler.test.js")
	require.NoError(t, os.WriteFile(src, []byte("test();"), 0o644))
	dest := filepath.Join(dir, "out.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))

	entries := zipEntries(t, dest)
	assert.Empty(t, entries, "a single-file source matching an exclude pattern must not be archived")
}

func TestRun_Replace_SingleFile_IncludeMustMatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "handler.js")
	require.NoError(t, os.WriteFile(src, []byte("x=1"), 0o644))
	dest := filepath.Join(dir, "out.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Include: []string{"**/*.test.js"}}))

	entries := zipEntries(t, dest)
	assert.Empty(t, entries, "a single-file source not matching any include pattern must not be archived")
}

func TestRun_Replace_RejectsSubpathTraversal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	tests := []string{"../evil", "../../evil", "a/../../b", "..\\evil", "/absolute"}
	for _, subpath := range tests {
		t.Run(subpath, func(t *testing.T) {
			err := Run(ActionReplace, &PackOptions{
				Source:      src,
				Destination: filepath.Join(dir, "out.zip"),
				Subpath:     subpath,
			})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrArchiveInvalidSubpath), "got %v", err)
		})
	}
}

func TestRun_Replace_AllowsOrdinaryRelativeSubpath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Subpath: "opt/nodejs"}))

	entries := zipEntries(t, dest)
	assert.Contains(t, entries, "opt/nodejs/handler.js")
}

func TestCollectDirEntries_PreservesInvalidGlobError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	_, err := collectEntries(src, "", nil, []string{"["})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveInvalidGlobPattern), "got %v", err)
	assert.False(t, errors.Is(err, errUtils.ErrArchiveSourceNotFound), "an invalid glob pattern must not be misclassified as a missing source")
}

func TestRun_Update_PreservesDestinationMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits are not meaningfully enforced on Windows")
	}

	tests := []struct {
		name string
		ext  string
	}{
		{"zip", ".zip"},
		{"tar", ".tar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "src")
			writeFixture(t, src)
			dest := filepath.Join(dir, "out"+tt.ext)

			require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))
			require.NoError(t, os.Chmod(dest, 0o640))

			require.NoError(t, os.WriteFile(filepath.Join(src, "handler.js"), []byte("exports.handler = 2;"), 0o644))
			require.NoError(t, Run(ActionUpdate, &PackOptions{Source: filepath.Join(src, "handler.js"), Destination: dest}))

			info, err := os.Stat(dest)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "update must preserve the destination's existing file mode")
		})
	}
}

func TestRun_Replace_DefaultModeWhenDestinationMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode bits are not meaningfully enforced on Windows")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")

	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest}))

	info, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(defaultArchivePerm), info.Mode().Perm())
}

func TestWriteZip_AtomicOnFailure(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.txt")
	dest := filepath.Join(dir, "out.zip")
	entries := []packEntry{{fsPath: missing, archivePath: "missing.txt"}}

	err := writeZip(dest, entries)
	require.Error(t, err)

	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "a failed write must not leave a truncated destination behind")
	assertNoLeftoverTempFiles(t, dir)
}

func TestWriteZip_AtomicOnFailure_PreservesExistingArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")
	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))
	before := zipEntries(t, dest)

	missing := filepath.Join(dir, "missing.txt")
	entries := []packEntry{{fsPath: missing, archivePath: "missing.txt"}}
	err := writeZip(dest, entries)
	require.Error(t, err)

	after := zipEntries(t, dest)
	assert.Equal(t, before, after, "a failed replace must leave the previous valid archive untouched")
	assertNoLeftoverTempFiles(t, dir)
}

func TestWriteTar_AtomicOnFailure(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.txt")
	dest := filepath.Join(dir, "out.tar")
	entries := []packEntry{{fsPath: missing, archivePath: "missing.txt"}}

	err := writeTar(dest, entries, false)
	require.Error(t, err)

	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "a failed write must not leave a truncated destination behind")
	assertNoLeftoverTempFiles(t, dir)
}

func TestWriteTar_AtomicOnFailure_PreservesExistingArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.tar")
	require.NoError(t, Run(ActionReplace, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))
	before := tarEntries(t, dest, false)

	missing := filepath.Join(dir, "missing.txt")
	entries := []packEntry{{fsPath: missing, archivePath: "missing.txt"}}
	err := writeTar(dest, entries, false)
	require.Error(t, err)

	after := tarEntries(t, dest, false)
	assert.Equal(t, before, after, "a failed replace must leave the previous valid archive untouched")
	assertNoLeftoverTempFiles(t, dir)
}

// assertNoLeftoverTempFiles fails the test if any archive temp file (the
// ".archive-write-*"/".archive-update-*" pattern used by the temp-file+rename
// write path) was left behind in dir.
func assertNoLeftoverTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), ".archive-"), "leftover temp file: %s", e.Name())
	}
}

func TestDetectFormat_InfersAllExtensions(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"out.tar.bz2", FormatTarBz2},
		{"out.tbz2", FormatTarBz2},
		{"out.tar.xz", FormatTarXz},
		{"out.txz", FormatTarXz},
		{"out.tar", FormatTar},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, err := DetectFormat("", tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRun_UnknownAction(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	err := Run(Action("bogus"), &PackOptions{Source: src, Destination: filepath.Join(dir, "out.zip")})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveActionNotImplemented))
}

func TestRun_Update_ValidationErrors(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		opts    *PackOptions
		wantErr error
	}{
		{
			name:    "unknown format",
			opts:    &PackOptions{Source: dir, Destination: filepath.Join(dir, "out.zip"), Format: "rar"},
			wantErr: errUtils.ErrArchiveUnknownFormat,
		},
		{
			name:    "source does not exist",
			opts:    &PackOptions{Source: filepath.Join(dir, "nope"), Destination: filepath.Join(dir, "out.zip")},
			wantErr: errUtils.ErrArchiveSourceNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Run(ActionUpdate, tt.opts)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), "got %v, want %v", err, tt.wantErr)
		})
	}
}

// notADirDestination returns a destination path nested under a regular file,
// so os.MkdirAll(filepath.Dir(destination)) is guaranteed to fail.
func notADirDestination(t *testing.T, dir string) string {
	t.Helper()
	notADir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))
	return filepath.Join(notADir, "sub", "out.zip")
}

func TestRun_Replace_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	err := Run(ActionReplace, &PackOptions{Source: src, Destination: notADirDestination(t, dir)})
	require.Error(t, err)
}

func TestRun_Update_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)

	err := Run(ActionUpdate, &PackOptions{Source: src, Destination: notADirDestination(t, dir)})
	require.Error(t, err)
}

func TestDestinationMode(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing returns default", func(t *testing.T) {
		mode, err := destinationMode(filepath.Join(dir, "nope.zip"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(defaultArchivePerm), mode)
	})

	if runtime.GOOS != "windows" {
		t.Run("existing returns its mode", func(t *testing.T) {
			f := filepath.Join(dir, "existing.zip")
			require.NoError(t, os.WriteFile(f, []byte("x"), 0o640))
			mode, err := destinationMode(f)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), mode)
		})
	}

	t.Run("stat error other than not-exist", func(t *testing.T) {
		notADir := filepath.Join(dir, "not-a-dir-mode")
		require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))
		_, err := destinationMode(filepath.Join(notADir, "out.zip"))
		require.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrArchiveWriteFailed))
	})
}

func TestAtomicRewrite_DestinationModeError(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))
	dest := filepath.Join(notADir, "out.zip")

	err := atomicRewrite(dest, ".archive-write-*.zip", func(tmp *os.File) error { return nil })
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveWriteFailed))
}

func TestAtomicRewrite_CreateTempFailure(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "does-not-exist-dir", "out.zip")

	err := atomicRewrite(dest, ".archive-write-*.zip", func(tmp *os.File) error { return nil })
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveWriteFailed))
}

func TestAtomicRewrite_WriteFuncError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")
	sentinel := errors.New("boom")

	err := atomicRewrite(dest, ".archive-write-*.zip", func(tmp *os.File) error { return sentinel })
	require.Error(t, err)
	assert.True(t, errors.Is(err, sentinel))
	assertNoLeftoverTempFiles(t, dir)
}

func TestAtomicRewrite_RenameFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming a file over a non-empty directory has different semantics on Windows")
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")
	require.NoError(t, os.Mkdir(dest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "keep.txt"), []byte("x"), 0o644))

	err := atomicRewrite(dest, ".archive-write-*.zip", func(tmp *os.File) error {
		_, werr := tmp.WriteString("data")
		return werr
	})
	require.Error(t, err)
}

func TestUpdateZip_MissingSourceEntry(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.zip")

	err := updateZip(dest, []packEntry{{fsPath: filepath.Join(dir, "missing.txt"), archivePath: "missing.txt"}})
	require.Error(t, err)
	assertNoLeftoverTempFiles(t, dir)
}

func TestUpdateTar_MissingSourceEntry(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.tar")

	err := updateTar(dest, []packEntry{{fsPath: filepath.Join(dir, "missing.txt"), archivePath: "missing.txt"}})
	require.Error(t, err)
	assertNoLeftoverTempFiles(t, dir)
}

func TestRun_Update_Zip_CorruptExistingArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.zip")
	require.NoError(t, os.WriteFile(dest, []byte("not a zip file"), 0o644))

	err := Run(ActionUpdate, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}})
	require.Error(t, err)
}

func TestRun_Update_Tar_CorruptExistingArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.tar")
	require.NoError(t, os.WriteFile(dest, []byte("not a tar file"), 0o644))

	err := Run(ActionUpdate, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}})
	require.Error(t, err)
}

func TestRun_Update_CreatesWhenDestinationMissing_Tar(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	writeFixture(t, src)
	dest := filepath.Join(dir, "out.tar")

	require.NoError(t, Run(ActionUpdate, &PackOptions{Source: src, Destination: dest, Exclude: []string{"**/*.test.js"}}))

	entries := tarEntries(t, dest, false)
	assert.Contains(t, entries, "handler.js")
}

func TestCopyUnchangedTarEntries_OpenFailure(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o644))
	dest := filepath.Join(notADir, "out.tar")

	err := copyUnchangedTarEntries(nil, dest, map[string]bool{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveWriteFailed))
}

func TestCollectEntries_SingleFile_InvalidGlobPattern(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "handler.js")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))

	_, err := collectEntries(src, "", nil, []string{"["})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveInvalidGlobPattern))
}

func TestMatchesFilters_InvalidIncludePattern(t *testing.T) {
	_, err := matchesFilters("foo.js", []string{"["}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveInvalidGlobPattern))
}

func TestCollectDirEntries_PropagatesGenericWalkError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	blocked := filepath.Join(src, "blocked")
	require.NoError(t, os.MkdirAll(blocked, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(blocked, "f.txt"), []byte("x"), 0o644))
	require.NoError(t, os.Chmod(blocked, 0o000))
	defer os.Chmod(blocked, 0o755) // Best-effort restore so t.TempDir() cleanup can remove it.

	_, err := collectEntries(src, "", nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveWalkFailed), "got %v", err)
	assert.False(t, errors.Is(err, errUtils.ErrArchiveInvalidGlobPattern))
}
