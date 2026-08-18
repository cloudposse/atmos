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

func TestCustomGCLBinaryPath(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "custom-gcl")
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	assert.Equal(t, want, customGCLBinaryPath(root))
}

// writeCustomGCLFixture builds a root tree with a .custom-gcl.yml and a
// tools/lintroller directory, returning the root and the path a caller
// should treat as the built binary (not created here).
func writeCustomGCLFixture(t *testing.T) (root, binPath string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".custom-gcl.yml"), []byte("plugins: []\n"), 0o644))
	lintrollerDir := filepath.Join(root, "tools", "lintroller")
	require.NoError(t, os.MkdirAll(lintrollerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lintrollerDir, "plugin.go"), []byte("package lintroller"), 0o644))
	return root, customGCLBinaryPath(root)
}

func TestCustomGCLIsStale(t *testing.T) {
	t.Run("binary missing", func(t *testing.T) {
		root, binPath := writeCustomGCLFixture(t)
		got, err := customGCLIsStale(root, binPath)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("config missing", func(t *testing.T) {
		root := t.TempDir()
		binPath := customGCLBinaryPath(root)
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		_, err := customGCLIsStale(root, binPath)
		require.Error(t, err)
	})

	t.Run("config newer than binary", func(t *testing.T) {
		root, binPath := writeCustomGCLFixture(t)
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		now := time.Now()
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))
		require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, now.Add(time.Hour)))

		got, err := customGCLIsStale(root, binPath)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("lintroller source newer than binary", func(t *testing.T) {
		root, binPath := writeCustomGCLFixture(t)
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		now := time.Now()
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))
		require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, now.Add(-time.Hour)))
		require.NoError(t, os.Chtimes(filepath.Join(root, "tools", "lintroller", "plugin.go"), time.Time{}, now.Add(time.Hour)))

		got, err := customGCLIsStale(root, binPath)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("everything older than binary", func(t *testing.T) {
		root, binPath := writeCustomGCLFixture(t)
		now := time.Now()
		require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, now.Add(-time.Hour)))
		require.NoError(t, os.Chtimes(filepath.Join(root, "tools", "lintroller", "plugin.go"), time.Time{}, now.Add(-time.Hour)))
		require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
		require.NoError(t, os.Chtimes(binPath, time.Time{}, now))

		got, err := customGCLIsStale(root, binPath)
		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestLintCustomGCLSkipsRebuildWhenUpToDate(t *testing.T) {
	root, binPath := writeCustomGCLFixture(t)
	now := time.Now()
	require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, now.Add(-time.Hour)))
	require.NoError(t, os.Chtimes(filepath.Join(root, "tools", "lintroller", "plugin.go"), time.Time{}, now.Add(-time.Hour)))
	require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
	require.NoError(t, os.Chtimes(binPath, time.Time{}, now))

	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	t.Chdir(root)

	before, err := os.Stat(binPath)
	require.NoError(t, err)

	require.NoError(t, Lint{}.CustomGCL())

	after, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(), "binary should not have been rebuilt")
}

// TestLintCustomGCLRebuildsWhenStale exercises the real rebuild branch by
// faking `golangci-lint` on PATH (a build-only stand-in, not a network call),
// so the orchestration logic (env vars, args passed to `golangci-lint
// custom`) gets real coverage without an actual clone+compile.
func TestLintCustomGCLRebuildsWhenStale(t *testing.T) {
	root, _ := writeCustomGCLFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	argsFile := setUpFakePathBinary(t, "golangci-lint")
	t.Chdir(root)

	require.NoError(t, Lint{}.CustomGCL())

	args := readFakeBinArgs(t, argsFile)
	assert.Equal(t, []string{"custom"}, args)
}

// TestLintCustomGCLBuildsFromRepoRoot reproduces the bug CodeRabbit flagged:
// `golangci-lint custom` previously inherited the caller's working
// directory, so `.custom-gcl.yml` and the `custom-gcl` output path silently
// diverged from the root-based binPath when invoked from a subdirectory. It
// must always build with root as its working directory, regardless of where
// `go tool mage` was invoked from.
func TestLintCustomGCLBuildsFromRepoRoot(t *testing.T) {
	root, _ := writeCustomGCLFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	argsFile := setUpFakePathBinary(t, "golangci-lint")

	nested := filepath.Join(root, "tools", "lintroller")
	t.Chdir(nested)

	require.NoError(t, Lint{}.CustomGCL())

	// The subprocess's own getcwd(2) resolves symlinks (e.g. macOS's
	// /var -> /private/var) that a parent-process os.Getwd() does not,
	// so resolve root the same way before comparing.
	wantRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	assert.Equal(t, wantRoot, readFakeBinCwd(t, argsFile))
}

// TestLintCustomGCLBuildFailurePropagates covers the wrapped-error branch
// when `golangci-lint custom` fails.
func TestLintCustomGCLBuildFailurePropagates(t *testing.T) {
	root, _ := writeCustomGCLFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	setUpFakePathBinary(t, "golangci-lint")
	t.Setenv(fakeBinExitEnv, "1")
	t.Chdir(root)

	err := Lint{}.CustomGCL()
	require.ErrorIs(t, err, errCustomGCLBuildFailed)
}
