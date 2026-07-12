package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitFixture builds a temp git repo with commits at controlled timestamps:
//   - src/a.txt added at t1
//   - src/nested/b.txt added at t2 (t2 > t1)
//   - README.md (outside src/) added at t3 (t3 > t2)
//
// Returns the repo root and the three commit timestamps, so tests can assert
// against exact expected values instead of just "not the real mtime".
func gitFixture(t *testing.T) (root string, t1, t2, t3 time.Time) {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	sig := func(when time.Time) *object.Signature {
		return &object.Signature{Name: "test", Email: "test@example.com", When: when}
	}
	commit := func(path, content string, when time.Time, msg string) {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
		_, err := wt.Add(path)
		require.NoError(t, err)
		_, err = wt.Commit(msg, &git.CommitOptions{Author: sig(when), Committer: sig(when)})
		require.NoError(t, err)
	}

	t1 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 = time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)
	t3 = time.Date(2022, 12, 25, 0, 0, 0, 0, time.UTC)

	commit("src/a.txt", "a", t1, "add a\n")
	commit("src/nested/b.txt", "b", t2, "add b\n")
	commit("README.md", "readme", t3, "add readme\n")

	return dir, t1, t2, t3
}

func TestNewReproducibleTimestamps_EmptyMode(t *testing.T) {
	dir := t.TempDir()
	rt := newReproducibleTimestamps("", dir)
	assert.True(t, rt.modTimeFor(filepath.Join(dir, "x")).IsZero(), "empty mode must signal 'don't override' via a zero time")
}

func TestNewReproducibleTimestamps_EpochMode_UsesLastCommitTouchingSubtree(t *testing.T) {
	root, _, t2, t3 := gitFixture(t)
	src := filepath.Join(root, "src")

	rt := newReproducibleTimestamps(ReproducibleEpoch, src)

	// Every file under src/ gets the SAME timestamp: the most recent commit
	// touching anything under src/ (t2, "add b"), not the repo-wide most
	// recent commit (t3, "add readme", which is outside src/ entirely).
	assert.Equal(t, t2, rt.modTimeFor(filepath.Join(src, "a.txt")))
	assert.Equal(t, t2, rt.modTimeFor(filepath.Join(src, "nested", "b.txt")))
	assert.NotEqual(t, t3, rt.epoch, "epoch must not leak the repo-wide latest commit outside src/")
}

func TestNewReproducibleTimestamps_EpochMode_SourceIsRepoRoot(t *testing.T) {
	root, _, _, t3 := gitFixture(t)

	// source == root: filepath.Rel(root, root) is ".", which must still match
	// every git path (not fail to match anything and silently fall back to
	// reproducibleFallbackEpoch).
	rt := newReproducibleTimestamps(ReproducibleEpoch, root)

	assert.Equal(t, t3, rt.modTimeFor(filepath.Join(root, "README.md")), "repo-root source must resolve the repo-wide latest commit, not fall back to the fixed epoch")
}

func TestNewReproducibleTimestamps_GitMode_UsesPerFileCommit(t *testing.T) {
	root, t1, t2, _ := gitFixture(t)
	src := filepath.Join(root, "src")

	rt := newReproducibleTimestamps(ReproducibleGit, src)

	assert.Equal(t, t1, rt.modTimeFor(filepath.Join(src, "a.txt")), "a.txt's own last commit is t1, not src/'s overall t2")
	assert.Equal(t, t2, rt.modTimeFor(filepath.Join(src, "nested", "b.txt")))
}

func TestNewReproducibleTimestamps_GitMode_FallsBackToEpochForUntrackedFiles(t *testing.T) {
	root, _, t2, _ := gitFixture(t)
	src := filepath.Join(root, "src")
	require.NoError(t, os.WriteFile(filepath.Join(src, "generated.js"), []byte("build output"), 0o644))

	rt := newReproducibleTimestamps(ReproducibleGit, src)

	// generated.js was never committed (simulates build output like
	// node_modules/ or a compiled binary) — falls back to the epoch value,
	// not the file's real (test-run-time) mtime.
	assert.Equal(t, t2, rt.modTimeFor(filepath.Join(src, "generated.js")))
}

func TestNewReproducibleTimestamps_FallsBackWhenNotInGitRepo(t *testing.T) {
	dir := t.TempDir() // plain temp dir, no .git anywhere above it in the tree
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))

	for _, mode := range []string{ReproducibleEpoch, ReproducibleGit} {
		t.Run(mode, func(t *testing.T) {
			rt := newReproducibleTimestamps(mode, src)
			assert.Equal(t, reproducibleFallbackEpoch, rt.modTimeFor(filepath.Join(src, "x.txt")))
		})
	}
}

func TestNewReproducibleTimestamps_FallsBackWhenSourceHasNoCommitHistory(t *testing.T) {
	dir := t.TempDir()
	_, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	// Repo exists but has zero commits — HEAD lookup itself fails.
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))

	rt := newReproducibleTimestamps(ReproducibleEpoch, src)
	assert.Equal(t, reproducibleFallbackEpoch, rt.modTimeFor(filepath.Join(src, "x.txt")))
}

func TestValidateReproducibleMode(t *testing.T) {
	for _, mode := range []string{"", ReproducibleEpoch, ReproducibleGit} {
		assert.NoError(t, validateReproducibleMode(mode), "mode %q should be valid", mode)
	}
	err := validateReproducibleMode("bogus")
	require.Error(t, err)
}

func TestNormalizeMode(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want os.FileMode
	}{
		{"regular file, umask 0022", 0o644, reproducibleFileMode},
		{"regular file, umask 0002 (group-writable)", 0o664, reproducibleFileMode},
		{"read-only file", 0o444, reproducibleFileMode},
		{"executable file", 0o755, reproducibleExecMode},
		{"executable, group-writable", 0o775, reproducibleExecMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeMode(tt.mode))
		})
	}
}
