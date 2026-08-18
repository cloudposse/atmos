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
