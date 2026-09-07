//go:build mage

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintrollerBinaryName(t *testing.T) {
	want := ".lintroller"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	assert.Equal(t, want, lintrollerBinaryName())
}

func TestLintrollerIsStale(t *testing.T) {
	t.Run("binary missing", func(t *testing.T) {
		dir := t.TempDir()
		got, err := lintrollerIsStale(dir, filepath.Join(dir, ".lintroller"))
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("newer go file makes it stale", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, ".lintroller")
		now := time.Now()
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "rule.go"), []byte("package lintroller"), 0o644))
		require.NoError(t, os.Chtimes(filepath.Join(dir, "rule.go"), time.Time{}, now.Add(time.Hour)))

		got, err := lintrollerIsStale(dir, binPath)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("newer go.mod makes it stale", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, ".lintroller")
		now := time.Now()
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
		require.NoError(t, os.Chtimes(filepath.Join(dir, "go.mod"), time.Time{}, now.Add(time.Hour)))

		got, err := lintrollerIsStale(dir, binPath)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("newer non-go file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, ".lintroller")
		now := time.Now()
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs"), 0o644))
		require.NoError(t, os.Chtimes(filepath.Join(dir, "README.md"), time.Time{}, now.Add(time.Hour)))

		got, err := lintrollerIsStale(dir, binPath)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("everything older than binary", func(t *testing.T) {
		dir := t.TempDir()
		binPath := filepath.Join(dir, ".lintroller")
		now := time.Now()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "rule.go"), []byte("package lintroller"), 0o644))
		require.NoError(t, os.Chtimes(filepath.Join(dir, "rule.go"), time.Time{}, now.Add(-time.Hour)))
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))

		got, err := lintrollerIsStale(dir, binPath)
		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestFilterOutTestdata(t *testing.T) {
	got := filterOutTestdata([]string{
		"github.com/cloudposse/atmos/pkg/foo",
		"",
		"github.com/cloudposse/atmos/pkg/foo/testdata",
		"github.com/cloudposse/atmos/internal/testdata/nested",
		"github.com/cloudposse/atmos/pkg/foo",
	})
	assert.Equal(t, []string{
		"github.com/cloudposse/atmos/pkg/foo",
		"github.com/cloudposse/atmos/pkg/foo",
	}, got, "duplicates are not deduped, only /testdata and empty entries are filtered")
}

func TestLintLintrollerPropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())
	err := Lint{}.Lintroller()
	require.ErrorIs(t, err, errMageRepoRootNotFound)
}

// TestLintLintrollerPropagatesStaleCheckError covers both the
// os.Stat-fails-for-a-reason-other-than-missing branch inside
// lintrollerIsStale itself and Lintroller's propagation of that error: making
// "tools" a regular file (not a directory) means os.Stat(binPath) fails with
// "not a directory" (ENOTDIR), which os.IsNotExist does not recognize as
// not-exist, so lintrollerIsStale must return it as a hard error rather than
// treating it as "binary missing".
func TestLintLintrollerPropagatesStaleCheckError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools"), []byte("not a directory"), 0o644))
	t.Chdir(root)

	err := Lint{}.Lintroller()
	require.Error(t, err)
	assert.False(t, os.IsNotExist(err), "the underlying stat error must not be a not-exist error for this test to exercise the intended branch")
}

// lintrollerFixture builds a self-contained root tree with a nested
// tools/lintroller Go module whose ./cmd/lintroller is a trivial program
// (not the real lintroller analyzer). This exercises Lintroller's real build
// orchestration (staleness -> `go build` -> chmod -> `go list` -> run)
// against a fast, dependency-free stand-in, rather than the real lintroller
// source, which is unit-tested independently under tools/lintroller.
func lintrollerFixture(t *testing.T, withMain bool) (root string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	lintrollerDir := filepath.Join(root, "tools", "lintroller")
	cmdDir := filepath.Join(lintrollerDir, "cmd", "lintroller")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lintrollerDir, "go.mod"),
		[]byte("module github.com/cloudposse/atmos/tools/lintroller\n\ngo 1.26\n"), 0o644))
	if withMain {
		require.NoError(t, os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644))
	}
	return root
}

// TestLintLintrollerBuildsWhenStale exercises the real rebuild branch end to
// end: no existing binary (stale), a real `go build` of a trivial stand-in
// program, the chmod that follows it, and the subsequent `go list` + binary
// run. This is the orchestration path CustomGCL/Precommit's tests fake via
// PATH, but here `go build`/`go list` are real (fast, dependency-free)
// invocations rather than faked, since faking them would hide whether the
// build actually produces a working, executable binary.
func TestLintLintrollerBuildsWhenStale(t *testing.T) {
	root := lintrollerFixture(t, true)
	t.Chdir(root)

	require.NoError(t, Lint{}.Lintroller())

	binPath := filepath.Join(root, "tools", "lintroller", lintrollerBinaryName())
	info, err := os.Stat(binPath)
	require.NoError(t, err, "expected go build to have produced the binary")
	if runtime.GOOS != "windows" {
		assert.NotZero(t, info.Mode().Perm()&0o100, "expected the binary to be chmod'd executable")
	}
}

// TestLintLintrollerBuildFails covers the wrapped-error branch when `go
// build` fails: an empty cmd/lintroller directory (no main.go) has nothing
// for `go build ./cmd/lintroller` to compile.
func TestLintLintrollerBuildFails(t *testing.T) {
	root := lintrollerFixture(t, false)
	t.Chdir(root)

	err := Lint{}.Lintroller()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build lintroller")
}

// TestLintLintrollerGoListError covers the `go list` error branch: the
// binary is already up to date (so the build branch is skipped), but the
// root go.mod is malformed beyond its first line, which mageRepoRoot never
// inspects but `go list` must fully parse.
func TestLintLintrollerGoListError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte(rootModuleDecl+"\n\nnot a valid go.mod directive\n"), 0o644))
	lintrollerDir := filepath.Join(root, "tools", "lintroller")
	require.NoError(t, os.MkdirAll(lintrollerDir, 0o755))
	binPath := filepath.Join(lintrollerDir, lintrollerBinaryName())
	require.NoError(t, os.WriteFile(binPath, []byte("stub"), 0o755))
	t.Chdir(root)

	err := Lint{}.Lintroller()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go list")
}

// TestLintLintrollerHappyPathWhenNotStale exercises the full non-rebuild
// path: an up-to-date lintroller binary, a faked `go list` (via PATH) whose
// output is filtered by filterOutTestdata, and a faked lintroller binary
// invocation (via its explicit binPath) — all without needing this repo's
// real tools/lintroller module or a real `go build`.
func TestLintLintrollerHappyPathWhenNotStale(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	lintrollerDir := filepath.Join(root, "tools", "lintroller")
	require.NoError(t, os.MkdirAll(lintrollerDir, 0o755))
	binPath := filepath.Join(lintrollerDir, lintrollerBinaryName())
	require.NoError(t, os.WriteFile(binPath, []byte("stub"), 0o755))

	setUpFakePathBinary(t, "go")
	t.Setenv(fakeBinStdoutEnv, "github.com/cloudposse/atmos/pkg/foo\ngithub.com/cloudposse/atmos/pkg/foo/testdata")
	argsFile := setUpFakeBin(t, binPath)

	t.Chdir(root)
	require.NoError(t, Lint{}.Lintroller())

	args := readFakeBinArgs(t, argsFile)
	assert.Equal(t, []string{"github.com/cloudposse/atmos/pkg/foo"}, args)
}
