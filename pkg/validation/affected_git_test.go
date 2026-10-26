package validation

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitInRepo runs git in dir with signing and identity pinned so tests never depend on the developer's git config.
func gitInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// newAffectedRepo creates a repository with one committed base commit on "main" and returns its (symlink-resolved) root.
func newAffectedRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	gitInRepo(t, root, "init", "-q", "-b", "main")
	writeRepoFile(t, root, "base.yaml", "a: 1\n")
	writeRepoFile(t, root, "stacks/keep.yaml", "a: 1\n")
	gitInRepo(t, root, "add", "-A")
	gitInRepo(t, root, "commit", "-q", "-m", "base")
	return root
}

func TestAffectedFiles_RealRepo(t *testing.T) {
	root := newAffectedRepo(t)
	t.Chdir(root)

	// Committed after the base (so it shows via base...HEAD).
	base := "HEAD"
	writeRepoFile(t, root, "committed.yaml", "a: 1\n")
	gitInRepo(t, root, "add", "committed.yaml")
	gitInRepo(t, root, "commit", "-q", "-m", "committed")

	// Staged only.
	writeRepoFile(t, root, "stacks/staged.yaml", "a: 1\n")
	gitInRepo(t, root, "add", "stacks/staged.yaml")

	// Unstaged modification of a tracked file.
	writeRepoFile(t, root, "stacks/keep.yaml", "a: 2\n")

	// Untracked, nested, and at the root.
	writeRepoFile(t, root, "stacks/orgs/untracked.yaml", "a: 1\n")
	writeRepoFile(t, root, "top-untracked.yaml", "a: 1\n")

	want := []string{"committed.yaml", "stacks/keep.yaml", "stacks/staged.yaml", "stacks/orgs/untracked.yaml", "top-untracked.yaml"}

	t.Run("from repo root", func(t *testing.T) {
		got, err := AffectedFiles(base + "~1")
		require.NoError(t, err)
		assert.ElementsMatch(t, want, got)
	})

	t.Run("from a subdirectory paths stay repo-root relative", func(t *testing.T) {
		t.Chdir(filepath.Join(root, "stacks", "orgs"))
		got, err := AffectedFiles(base + "~1")
		require.NoError(t, err)
		assert.ElementsMatch(t, want, got)
	})
}

func TestAffectedFiles_StagedOnlyChangeIsDetected(t *testing.T) {
	root := newAffectedRepo(t)
	t.Chdir(root)

	writeRepoFile(t, root, "stacks/only-staged.yaml", "a: 1\n")
	gitInRepo(t, root, "add", "stacks/only-staged.yaml")

	got, err := AffectedFiles("HEAD")
	require.NoError(t, err)
	assert.Equal(t, []string{"stacks/only-staged.yaml"}, got)
}

func TestAffectedFiles_CleanRepoHasNoChanges(t *testing.T) {
	root := newAffectedRepo(t)
	t.Chdir(root)

	got, err := AffectedFiles("HEAD")
	require.NoError(t, err)
	assert.Empty(t, got)
}
