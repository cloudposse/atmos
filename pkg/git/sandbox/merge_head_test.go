package sandbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pendingMerge leaves root with a clean merge staged but not yet committed.
func pendingMerge(t *testing.T) string {
	t.Helper()
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "base.txt"), "base\n")
	commit(t, root)
	git(t, root, "checkout", "-q", "-b", "other")
	mustWrite(t, filepath.Join(root, "other.txt"), "other\n")
	commit(t, root)
	git(t, root, "checkout", "-q", "-")
	mustWrite(t, filepath.Join(root, "main.txt"), "main\n")
	commit(t, root)
	git(t, root, "merge", "--no-commit", "--no-ff", "-q", "other")
	return root
}

func TestSandboxCarriesPendingMerge(t *testing.T) {
	root := pendingMerge(t)
	source := strings.TrimSpace(git(t, root, "rev-parse", "--verify", "MERGE_HEAD"))
	require.NotEmpty(t, source)

	box, err := Open(t.Context(), Options{Directory: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, box.Close()) })

	// Checks must see the commit being made as a merge, with the same second parent.
	require.Equal(t, source, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", "MERGE_HEAD")))
	// The merged-in objects stay reachable through the read-only alternates.
	require.Equal(t, source, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", "MERGE_HEAD^{commit}")))
	// The caller's own merge state is untouched.
	require.Equal(t, source, strings.TrimSpace(git(t, root, "rev-parse", "--verify", "MERGE_HEAD")))
}

func TestSandboxDoesNotInventMerge(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file.txt"), "one\n")
	commit(t, root)

	box, err := Open(t.Context(), Options{Directory: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, box.Close()) })

	_, err = gitOutput(t.Context(), box.Path, "rev-parse", "-q", "--verify", "MERGE_HEAD")
	require.Error(t, err, "an ordinary commit must not look like a merge")
}

// pendingOctopusMerge stages two independent incoming branches after a local commit.
func pendingOctopusMerge(t *testing.T, root string, secondFiles int) []string {
	t.Helper()
	mustWrite(t, filepath.Join(root, "base.txt"), "base\n")
	commit(t, root)
	base := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	heads := make([]string, 0, 2)
	for i, count := range []int{1, secondFiles} {
		name := "incoming-" + strconv.Itoa(i)
		git(t, root, "checkout", "-q", "-b", name, base)
		for j := range count {
			mustWrite(t, filepath.Join(root, name, strconv.Itoa(j)), "incoming\n")
		}
		commit(t, root)
		heads = append(heads, strings.TrimSpace(git(t, root, "rev-parse", "HEAD")))
	}
	git(t, root, "checkout", "-q", "-b", "feature", base)
	mustWrite(t, filepath.Join(root, "own.txt"), "branch change\n")
	commit(t, root)
	git(t, root, "merge", "--no-commit", "--no-ff", heads[0], heads[1])
	return heads
}

func TestSandboxCarriesEveryMergeHead(t *testing.T) {
	root := newRepo(t)
	heads := pendingOctopusMerge(t, root, 1)
	sourcePath := strings.TrimSpace(git(t, root, "rev-parse", "--path-format=absolute", "--git-path", "MERGE_HEAD"))
	before, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	require.Equal(t, strings.Join(heads, "\n")+"\n", string(before))
	box := openStaged(t, root, heads[1])
	copied, err := os.ReadFile(filepath.Join(box.Path, ".git", "MERGE_HEAD"))
	require.NoError(t, err)
	require.Equal(t, before, copied)
	for _, head := range heads {
		require.Equal(t, head, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", head+"^{commit}")))
	}
	after, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestCopyMergeHeadsFailures(t *testing.T) {
	t.Run("destination write failure", func(t *testing.T) {
		root := pendingMerge(t)
		// The destination has no Git metadata directory.
		err := copyMergeHeads(t.Context(), root, t.TempDir())
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("unreadable merge metadata blocks snapshot", func(t *testing.T) {
		root := newRepo(t)
		mustWrite(t, filepath.Join(root, "file"), "base\n")
		commit(t, root)
		// A directory is unreadable as a pseudoref, including when tests run as root.
		require.NoError(t, os.Mkdir(filepath.Join(root, ".git", "MERGE_HEAD"), 0o700))
		box, err := Open(t.Context(), Options{Directory: root})
		require.ErrorContains(t, err, "read pending merge heads")
		require.Nil(t, box)
	})
}
