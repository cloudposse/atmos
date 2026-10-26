package sandbox

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func openStaged(t *testing.T, root, baseRef string) *Sandbox {
	t.Helper()
	box, err := Open(t.Context(), Options{Directory: root, BaseRef: baseRef})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, box.Close()) })
	return box
}

func TestSandboxCopiesRequestedBaseRefOnly(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file.txt"), "one\n")
	commit(t, root)
	head := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	// A stacked PR compares against its parent branch, not origin/main.
	git(t, root, "update-ref", "refs/remotes/upstream/parent", head)
	git(t, root, "update-ref", "refs/remotes/origin/main", head)

	box := openStaged(t, root, "upstream/parent")

	require.Equal(t, head, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", "refs/remotes/upstream/parent")))
	_, err := gitOutput(t.Context(), box.Path, "rev-parse", "-q", "--verify", "refs/remotes/origin/main")
	require.Error(t, err, "the sandbox must not hardcode origin/main when another base was requested")
}

func TestSandboxDefaultBaseIsOriginMain(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file.txt"), "one\n")
	commit(t, root)
	head := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	git(t, root, "update-ref", "refs/remotes/origin/main", head)

	box := openStaged(t, root, "")

	require.Equal(t, head, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", "origin/main")))
}

func TestSandboxToleratesBasesWithoutARef(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file.txt"), "one\n")
	commit(t, root)
	head := strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
	for name, base := range map[string]string{
		"commit SHA":      head,
		"missing ref":     "does/not-exist",
		"option-like ref": "--exec=true",
	} {
		t.Run(name, func(t *testing.T) {
			box := openStaged(t, root, base)
			// No ref is invented for these; HEAD is still copied.
			require.Equal(t, head, strings.TrimSpace(git(t, box.Path, "rev-parse", "--verify", "HEAD")))
		})
	}
}
