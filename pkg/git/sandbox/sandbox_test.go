package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxTracksModuleSideEffectsAndCopiesSupport(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "nested", "main.go"), "package nested\n")
	commit(t, root)
	mustWrite(t, filepath.Join(root, "nested", "main.go"), "package nested // staged\n")
	git(t, root, "add", "nested/main.go")
	mustWrite(t, filepath.Join(root, "linter"), "prebuilt")
	timestamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(filepath.Join(root, "linter"), timestamp, timestamp))
	before := git(t, root, "ls-files", "-s")
	box, err := Open(t.Context(), Options{Directory: filepath.Join(root, "nested")})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, box.Close()) })
	require.NoError(t, box.Copy([]string{"linter"}))
	info, err := os.Stat(filepath.Join(box.Path, "linter"))
	require.NoError(t, err)
	require.Equal(t, timestamp, info.ModTime())
	require.Error(t, box.Copy([]string{"linter"}))
	require.Error(t, box.Copy([]string{"nested/main.go"}))
	require.Error(t, box.Copy([]string{"../escape"}))
	require.Error(t, box.Copy([]string{"missing"}))
	require.NoError(t, box.Track([]string{"nested/./go.sum", "nested/go.sum"}))
	mustWrite(t, filepath.Join(box.Path, "nested", "go.sum"), "generated\n")
	mustWrite(t, filepath.Join(box.Path, "linter"), "modified support is not returned")
	changed, err := box.Finish(true)
	require.NoError(t, err)
	require.True(t, changed)
	data, err := os.ReadFile(filepath.Join(root, "nested", "go.sum"))
	require.NoError(t, err)
	require.Equal(t, "generated\n", string(data))
	data, err = os.ReadFile(filepath.Join(root, "linter"))
	require.NoError(t, err)
	require.Equal(t, "prebuilt", string(data))
	require.Equal(t, before, git(t, root, "ls-files", "-s"))
	require.Error(t, box.Track([]string{"more"}))
	require.Error(t, box.Copy([]string{"linter"}))
	_, err = box.Finish(true)
	require.Error(t, err)
	require.NoError(t, box.Close())
	require.NoError(t, box.Close())
	_, err = os.Stat(box.Path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSandboxReadOnlyAndCanceledFinish(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "read only", true: "cancelled"}[cancelRun], func(t *testing.T) {
			root := newRepo(t)
			mustWrite(t, filepath.Join(root, "file"), "staged\n")
			git(t, root, "add", ".")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			box, err := Open(ctx, Options{Directory: root})
			require.NoError(t, err)
			defer box.Close()
			mustWrite(t, filepath.Join(box.Path, "file"), "fixed\n")
			if cancelRun {
				cancel()
			}
			changed, err := box.Finish(cancelRun)
			if cancelRun {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
				require.True(t, changed)
			}
			data, err := os.ReadFile(filepath.Join(root, "file"))
			require.NoError(t, err)
			require.Equal(t, "staged\n", string(data))
		})
	}
}

func TestSandboxValidationAndUnmergedIndex(t *testing.T) {
	root := newRepo(t)
	_, err := Open(t.Context(), Options{Directory: root, Scope: "unknown"})
	require.ErrorContains(t, err, "scope")
	_, err = Open(t.Context(), Options{Directory: root, Scope: "pr", BaseRef: "absent"})
	require.ErrorContains(t, err, "base ref")
	mustWrite(t, filepath.Join(root, "file"), "baseline\n")
	commit(t, root)
	hash := strings.TrimSpace(git(t, root, "rev-parse", "HEAD:file"))
	_, err = gitInput(t.Context(), root, strings.NewReader("100644 "+hash+" 1\tconflict\x00"), "update-index", "-z", "--index-info")
	require.NoError(t, err)
	index := git(t, root, "ls-files", "-s")
	_, err = Open(t.Context(), Options{Directory: root})
	require.ErrorContains(t, err, "unmerged")
	require.Equal(t, index, git(t, root, "ls-files", "-s"))
}

func TestSandboxRejectsRootAndGitMetadata(t *testing.T) {
	root := newRepo(t)
	mustWrite(t, filepath.Join(root, "file"), "content\n")
	git(t, root, "add", ".")
	box, err := Open(t.Context(), Options{Directory: root})
	require.NoError(t, err)
	defer box.Close()
	for _, path := range []string{".", "nested/..", ".git/index", "nested/.git/config", ".GIT/index", "../escape"} {
		require.Error(t, box.Track([]string{path}), path)
		require.Error(t, box.Copy([]string{path}), path)
	}
}
