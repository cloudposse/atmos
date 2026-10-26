package sandbox

import (
	"path/filepath"
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
