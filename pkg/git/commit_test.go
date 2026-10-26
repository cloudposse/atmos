package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commitTestFile writes a file and commits it via the git CLI, returning the SHA.
func commitTestFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", msg)
	return strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
}

func TestCommitParents(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	initial := commitTestFile(t, dir, "a.txt", "a", "initial")
	second := commitTestFile(t, dir, "b.txt", "b", "second")

	t.Run("initial commit has no parents and no error", func(t *testing.T) {
		sha, parents, err := CommitParents(dir, initial)
		require.NoError(t, err)
		assert.Equal(t, initial, sha)
		assert.Empty(t, parents, "an initial commit is valid — empty parents, nil error")
	})

	t.Run("HEAD resolves with its parent", func(t *testing.T) {
		sha, parents, err := CommitParents(dir, "HEAD")
		require.NoError(t, err)
		assert.Equal(t, second, sha)
		require.Len(t, parents, 1)
		assert.Equal(t, initial, parents[0])
	})

	t.Run("merge commit has two parents in order", func(t *testing.T) {
		runGit(t, dir, "checkout", "-b", "feature", initial)
		featureSHA := commitTestFile(t, dir, "c.txt", "c", "feature commit")
		runGit(t, dir, "checkout", "main")
		runGit(t, dir, "merge", "--no-ff", featureSHA, "-m", "merge feature")
		mergeSHA := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))

		sha, parents, err := CommitParents(dir, mergeSHA)
		require.NoError(t, err)
		assert.Equal(t, mergeSHA, sha)
		require.Len(t, parents, 2)
		assert.Equal(t, second, parents[0], "first parent is the pre-merge target tip")
		assert.Equal(t, featureSHA, parents[1], "second parent is the merged branch head")
	})

	t.Run("unresolvable revision errors", func(t *testing.T) {
		_, _, err := CommitParents(dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		assert.Error(t, err)
	})

	// NOTE: resolveCommit's CommitObject-fails-after-ResolveRevision-succeeds
	// branch (the "getting commit %s" error) is not covered by a test: go-git's
	// ResolveRevision already validates the target is commit-ish internally —
	// resolving a tree object's hex SHA (verified experimentally) fails at
	// ResolveRevision itself with "reference not found", never reaching
	// CommitObject. That branch is unreachable via any real revision string
	// without corrupting the local object store, so it is intentionally left
	// untested (defensive code, not a gap in real-world coverage).
}

// TestCommitParents_OpenRepoFails verifies that CommitParents surfaces a
// wrapped error (rather than panicking or silently succeeding) when repoDir
// is not inside any git repository.
func TestCommitParents_OpenRepoFails(t *testing.T) {
	dir := t.TempDir() // No `git init` — not a repository.

	_, _, err := CommitParents(dir, "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening local repo")
}

func TestMergeBaseSHAs(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")

	forkPoint := commitTestFile(t, dir, "a.txt", "a", "fork point")
	runGit(t, dir, "checkout", "-b", "feature")
	featureSHA := commitTestFile(t, dir, "f.txt", "f", "feature commit")
	runGit(t, dir, "checkout", "main")
	mainSHA := commitTestFile(t, dir, "m.txt", "m", "main moved forward")

	t.Run("finds fork point between two SHAs", func(t *testing.T) {
		sha, err := MergeBaseSHAs(dir, featureSHA, mainSHA)
		require.NoError(t, err)
		assert.Equal(t, forkPoint, sha)
	})

	t.Run("accepts HEAD as a revision", func(t *testing.T) {
		// HEAD is on main.
		sha, err := MergeBaseSHAs(dir, "HEAD", featureSHA)
		require.NoError(t, err)
		assert.Equal(t, forkPoint, sha)
	})

	t.Run("unresolvable revision errors", func(t *testing.T) {
		_, err := MergeBaseSHAs(dir, featureSHA, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		assert.Error(t, err)
	})

	// Distinct from the revB case above: revA is the one that fails to resolve.
	t.Run("unresolvable revA errors", func(t *testing.T) {
		_, err := MergeBaseSHAs(dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", featureSHA)
		assert.Error(t, err)
	})

	// Two orphan histories in the same repo are each individually resolvable
	// but share no common ancestor — MergeBase succeeds without error yet
	// returns zero results, which MergeBaseSHAs must turn into ErrNoCommonAncestor
	// rather than an empty-but-successful SHA.
	t.Run("no common ancestor", func(t *testing.T) {
		runGit(t, dir, "checkout", "--orphan", "unrelated")
		runGit(t, dir, "rm", "-rf", ".")
		unrelatedSHA := commitTestFile(t, dir, "u.txt", "u", "unrelated orphan commit")

		_, err := MergeBaseSHAs(dir, unrelatedSHA, mainSHA)
		require.ErrorIs(t, err, ErrNoCommonAncestor)
	})
}

// TestMergeBaseSHAs_OpenRepoFails verifies that MergeBaseSHAs surfaces a
// wrapped error when repoDir is not inside any git repository.
func TestMergeBaseSHAs_OpenRepoFails(t *testing.T) {
	dir := t.TempDir() // No `git init` — not a repository.

	_, err := MergeBaseSHAs(dir, "HEAD", "HEAD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening local repo")
}

// gitObjectExists reports whether sha resolves to a commit object already
// present in dir's local object database (loose or packed), without requiring
// any ref to point at it.
func gitObjectExists(dir, sha string) bool {
	cmd := exec.Command("git", gitTestArgs("cat-file", "-e", sha+"^{commit}")...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func TestFetchCommit(t *testing.T) {
	originDir := t.TempDir()
	runGit(t, originDir, "init", "-b", "main")
	commitTestFile(t, originDir, "a.txt", "a", "origin initial commit")
	// Local path remotes reject want-by-SHA unless explicitly allowed —
	// hosted forges (GitHub et al.) allow reachable SHAs.
	runGit(t, originDir, "config", "uploadpack.allowAnySHA1InWant", "true")

	cloneDir := t.TempDir()
	runGit(t, cloneDir, "clone", originDir, ".")

	t.Run("fetches an existing commit by SHA", func(t *testing.T) {
		// Commit the target object on origin AFTER cloneDir already exists, so the
		// clone genuinely lacks it — exercising the real "narrow checkout missing
		// an object" recovery path FetchCommit exists for. Committing it before the
		// clone (as the original version of this test did) would make the clone
		// already contain the object, so the assertion would pass even if
		// FetchCommit were a no-op.
		targetSHA := commitTestFile(t, originDir, "b.txt", "b", "origin commit added after clone")

		require.False(t, gitObjectExists(cloneDir, targetSHA),
			"target commit must be missing from the clone before FetchCommit for this test to be meaningful")

		err := FetchCommit(cloneDir, targetSHA)
		assert.NoError(t, err)

		assert.True(t, gitObjectExists(cloneDir, targetSHA),
			"target commit should be resolvable in the clone after FetchCommit")
	})

	t.Run("rejects a malformed SHA before invoking git", func(t *testing.T) {
		err := FetchCommit(cloneDir, "not-a-sha; rm -rf /")
		assert.ErrorIs(t, err, ErrInvalidCommitSHA)
	})

	t.Run("errors on an unknown commit", func(t *testing.T) {
		err := FetchCommit(cloneDir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
		assert.Error(t, err)
	})
}
