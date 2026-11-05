//go:build mage

package lint

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/mageutil"
	"github.com/cloudposse/atmos/magefiles/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLintPrecommitPropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())
	err := Lint{}.Precommit()
	require.ErrorIs(t, err, mageutil.ErrRepoRootNotFound)
}

func TestLintPrecommitMissingBinary(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(mageutil.RootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	t.Chdir(root)

	err := Lint{}.Precommit()
	require.Error(t, err)
}

// TestLintPrecommitStaleCheckError covers the customGCLIsStale error branch
// inside Precommit: the binary exists (os.Stat succeeds) but .custom-gcl.yml
// is missing, which customGCLIsStale always treats as a hard error.
func TestLintPrecommitStaleCheckError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(mageutil.RootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	binPath := customGCLBinaryPath(root)
	require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))
	t.Chdir(root)

	err := Lint{}.Precommit()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".custom-gcl.yml")
}

func TestLintPrecommitHappyPath(t *testing.T) {
	root := testutil.InitGitRepoFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
	testutil.RunGit(t, root, "add", "main.go")

	// customGCLIsStale (exercised by Precommit as of the staleness check
	// below) needs .custom-gcl.yml and tools/lintroller to exist and predate
	// the binary, or it treats the binary as stale/errors before ever
	// reaching runGolangciLintPrecommit.
	past := time.Now().Add(-time.Hour)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".custom-gcl.yml"), []byte("plugins: []\n"), 0o644))
	require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, past))
	lintrollerDir := filepath.Join(root, "tools", "lintroller")
	require.NoError(t, os.MkdirAll(lintrollerDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(lintrollerDir, "plugin.go"), []byte("package lintroller"), 0o644))
	require.NoError(t, os.Chtimes(filepath.Join(lintrollerDir, "plugin.go"), time.Time{}, past))

	binPath := customGCLBinaryPath(root)
	argsFile := testutil.SetUpFakeBin(t, binPath)
	t.Setenv("ATMOS_LINT_SHARED_CACHE", "1")
	t.Chdir(root)

	require.NoError(t, Lint{}.Precommit())

	args := testutil.ReadFakeBinArgs(t, argsFile)
	assert.Contains(t, args, "run")
}

// TestLintPrecommitStaleBinary reproduces the bug CodeRabbit flagged:
// Precommit previously only checked that custom-gcl existed, so a stale
// (pre-dependency-bump) binary would silently lint with outdated rules. It
// must now refuse to run and never invoke the stale binary.
func TestLintPrecommitStaleBinary(t *testing.T) {
	root := testutil.InitGitRepoFixture(t)
	binPath := customGCLBinaryPath(root)
	argsFile := testutil.SetUpFakeBin(t, binPath)

	// .custom-gcl.yml newer than the binary makes customGCLIsStale true.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".custom-gcl.yml"), []byte("plugins: []\n"), 0o644))
	require.NoError(t, os.Chtimes(filepath.Join(root, ".custom-gcl.yml"), time.Time{}, time.Now().Add(time.Hour)))
	t.Chdir(root)

	err := Lint{}.Precommit()
	require.Error(t, err)

	_, statErr := os.Stat(argsFile)
	assert.True(t, os.IsNotExist(statErr), "golangci-lint must not run against a stale custom-gcl binary")
}
