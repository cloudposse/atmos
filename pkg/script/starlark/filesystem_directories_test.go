package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestDirectoryOperations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	run := func(source string) (script.Result, error) {
		return New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: source})
	}
	_, err := run(`fs.mkdir("nested/child", parents=True)
fs.mkdir("nested/child", parents=True)
fs.write_file("nested/child/data", "payload")
fs.mkdir("single", mode=0o755)
fs.remove("single")`)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "nested", "child", "data"))
	for _, source := range []string{
		`fs.mkdir(42)`, `fs.mkdir("")`, `fs.mkdir("bad", mode=-1)`, `fs.mkdir("bad", mode=0o1000)`,
		`fs.mkdir("missing/child")`, `fs.mkdir("nested")`,
		`fs.mkdtemp(prefix=42)`, `fs.mkdtemp(dir="missing")`, `fs.mkdtemp(prefix="bad/name")`,
		`fs.remove(42)`, `fs.remove("")`, `fs.remove("missing")`, `fs.remove("nested")`,
	} {
		_, err := run(source)
		require.Error(t, err, source)
	}
	_, err = run(`fs.remove("nested", recursive=True)
fs.remove("nested", recursive=True)`)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(root, "nested"))
	volumeRoot := filepath.VolumeName(root) + string(filepath.Separator)
	_, err = New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Env: map[string]string{"ROOT": volumeRoot}, Source: `fs.remove(env["ROOT"], recursive=True)`})
	require.ErrorContains(t, err, "filesystem root")
}

func TestTemporaryDirectoryCleanupAfterFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: `path = fs.mkdtemp(prefix="fixture-", dir=".")
defer(fs.remove, path, recursive=True)
fs.write_file(path + "/data", "payload")
fail("intentional fixture failure")`})
	require.ErrorContains(t, err, "intentional fixture failure")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries, "deferred cleanup runs even after a failing assertion")
	result, err := New().Execute(t.Context(), script.Spec{Source: `path = fs.mkdtemp()
defer(fs.remove, path, recursive=True)
output = fs.stat(path).is_dir`})
	require.NoError(t, err)
	require.Equal(t, "true", result.Value)
}

func TestDirectoryOperationsDoNotFollowRemovalSymlink(t *testing.T) {
	t.Parallel()
	root, target := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "keep"), []byte("keep"), 0o600))
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: `fs.remove("link", recursive=True)`})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(target, "keep"))
	_, err = os.Lstat(filepath.Join(root, "link"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestDirectoryOperationsDryRunAndCancellation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`fs.mkdir("unexpected")`, `fs.mkdtemp(dir=".")`, `fs.remove("keep", recursive=True)`} {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "keep"), nil, 0o600))
		_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: source, DryRun: true})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = New().Execute(ctx, script.Spec{WorkingDirectory: root, Source: source})
		require.ErrorIs(t, err, context.Canceled)
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		require.Equal(t, "keep", entries[0].Name())
	}
}
