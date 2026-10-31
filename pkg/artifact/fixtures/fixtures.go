// Package fixtures builds the artifact test fixtures described by the artifact validation plan
// ("File package" and "Invalid package") with Go code inside a caller-supplied temporary
// directory. It never shells out, so it behaves identically on every platform.
package fixtures

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Permission bits used by the fixtures.
const (
	dirPerm  = 0o755
	tempPerm = 0o600
	execPerm = 0o755
	filePerm = 0o644
	privPerm = 0o640
)

// Well-known fixture content.
const (
	// RootReadme is the content of the root README.md, which stays ordinary payload.
	RootReadme = "# Root readme\n"

	// PackageReadme is the content of the distribution README in the reserved directory.
	PackageReadme = "# Package readme\n"

	// PackageChangelog is the content of the distribution CHANGELOG in the reserved directory.
	PackageChangelog = "# Changelog\n\n- first\n"

	// HelperScript is the content of the executable helper.
	HelperScript = "#!/bin/sh\necho helper\n"

	// UnicodeName is the Unicode filename in the file package.
	UnicodeName = "unicode-é-日本.txt"

	// ReservedDir is the reserved distribution directory below the source root.
	ReservedDir = ".atmos/artifact"
)

// POSIXModes reports whether the platform honors Unix permission bits. Windows does not, so tests
// that assert executable bits or mode-only changes skip there.
func POSIXModes() bool {
	defer perf.Track(nil, "fixtures.POSIXModes")()

	return runtime.GOOS != "windows"
}

// RequirePOSIXModes skips the test on platforms that ignore Unix permission bits.
func RequirePOSIXModes(t testing.TB) {
	defer perf.Track(nil, "fixtures.RequirePOSIXModes")()

	t.Helper()
	if !POSIXModes() {
		t.Skip("this platform does not honor Unix permission bits")
	}
}

// WriteFile writes content at the slash-separated rel path below dir and sets mode explicitly so
// the result does not depend on the process umask.
func WriteFile(t testing.TB, dir, rel string, content string, mode fs.FileMode) {
	defer perf.Track(nil, "fixtures.WriteFile")()

	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), dirPerm))
	require.NoError(t, os.WriteFile(path, []byte(content), tempPerm))
	require.NoError(t, os.Chmod(path, mode))
}

// MkdirAll creates the slash-separated directory below dir with mode 0755.
func MkdirAll(t testing.TB, dir, rel string) {
	defer perf.Track(nil, "fixtures.MkdirAll")()

	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(path, dirPerm))
	require.NoError(t, os.Chmod(path, dirPerm))
}

// Symlink creates a symlink at rel pointing at target and reports false (leaving nothing behind)
// when the platform or account cannot create symlinks.
func Symlink(t testing.TB, dir, rel, target string) bool {
	defer perf.Track(nil, "fixtures.Symlink")()

	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), dirPerm))
	if err := os.Symlink(filepath.FromSlash(target), path); err != nil {
		t.Logf("symlinks are unavailable here: %v", err)
		return false
	}
	return true
}

// FilePackage describes the files written by [WriteFilePackage].
type FilePackage struct {
	// Dir is the source directory.
	Dir string

	// Symlinks reports whether the confined symlink could be created.
	Symlinks bool
}

// WriteFilePackage writes the valid "File package" fixture into dir: nested files, an executable
// helper, an empty file, an empty directory, a confined symlink, a Unicode filename, a root README
// (ordinary payload), and a distinct package README and CHANGELOG in the reserved directory.
func WriteFilePackage(t testing.TB, dir string) FilePackage {
	defer perf.Track(nil, "fixtures.WriteFilePackage")()

	t.Helper()
	WriteFile(t, dir, "README.md", RootReadme, filePerm)
	WriteFile(t, dir, "bin/helper.sh", HelperScript, execPerm)
	WriteFile(t, dir, "nested/deep/file.txt", "deep content\n", filePerm)
	WriteFile(t, dir, "empty.txt", "", filePerm)
	MkdirAll(t, dir, "emptydir")
	WriteFile(t, dir, UnicodeName, "unicode content\n", privPerm)
	WriteFile(t, dir, ReservedDir+"/README.md", PackageReadme, filePerm)
	WriteFile(t, dir, ReservedDir+"/CHANGELOG.md", PackageChangelog, filePerm)
	symlinks := Symlink(t, dir, "link-to-file", "nested/deep/file.txt")
	return FilePackage{Dir: dir, Symlinks: symlinks}
}

// FilePackagePaths lists the inventory paths of the file package in sorted order, excluding the
// reserved distribution directory and, when symlinks are unavailable, the symlink.
func FilePackagePaths(symlinks bool) []string {
	defer perf.Track(nil, "fixtures.FilePackagePaths")()

	paths := []string{
		"README.md",
		"bin",
		"bin/helper.sh",
		"empty.txt",
		"emptydir",
		"nested",
		"nested/deep",
		"nested/deep/file.txt",
		UnicodeName,
	}
	if symlinks {
		paths = append(paths, "link-to-file")
	}
	sort.Strings(paths)
	return paths
}
