package starlark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestFileSystemMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "café.md"), []byte("café"), 0o600))
	result, err := New(WithFileSystem(automation.LocalFileSystem{})).Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, Env: map[string]string{"DIR": dir}, Source: `
def inspect():
    info = fs.stat("café.md")
    return [fs.glob("*.md"), info.size, info.is_file, info.is_dir, info.is_symlink, fs.exists("missing"), fs.glob("none*")]
output = [steps.parallel(functions=[inspect])[0], len(fs.glob(env["DIR"] + "/*.md"))]
`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[[["café.md"],5,true,false,false,false,[]],1]`, result.Value)
}

func TestFileSystemMetadataErrors(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`fs.glob("[")`, `fs.stat("missing")`, `fs.readlink("missing")`, `fs.glob()`, `fs.exists(1)`, `fs.stat(".", follow_symlinks="yes")`, `fs.readlink()`, `fs.stat(".").size = 3`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := New().Execute(context.Background(), script.Spec{WorkingDirectory: t.TempDir(), Source: source})
			require.Error(t, err)
		})
	}
	_, err := New().Execute(context.Background(), script.Spec{Source: `fs.stat("missing")`, DryRun: true})
	require.NoError(t, err)
}

func TestFileSystemSymlinkMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	result, err := New().Execute(context.Background(), script.Spec{WorkingDirectory: dir, Source: `output = [fs.exists("link"), fs.readlink("link"), fs.stat("link", follow_symlinks=False).is_symlink]`})
	require.NoError(t, err)
	assert.JSONEq(t, `[false,"missing",true]`, result.Value)
}

// setTestHome points the home directory at a temp dir and resets the homedir cache.
// Tests using it must not run in parallel because they modify process environment.
func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	return home
}

func TestFileSystemResolve(t *testing.T) {
	home := setTestHome(t)
	dir := t.TempDir()
	abs := filepath.Join(dir, "elsewhere", "file")
	result, err := New().Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, Env: map[string]string{"ABS": abs}, Source: `
output = [fs.resolve("~"), fs.resolve("~/a/../b"), fs.resolve("rel"), fs.resolve(env["ABS"]), fs.resolve("missing/../x"), fs.resolve("~user")]
`,
	})
	require.NoError(t, err)
	want, err := json.Marshal([]string{
		home, filepath.Join(home, "b"), filepath.Join(dir, "rel"), abs, filepath.Join(dir, "x"), filepath.Join(dir, "~user"),
	})
	require.NoError(t, err)
	assert.JSONEq(t, string(want), result.Value)
}

func TestFileSystemResolveErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ source, want string }{
		{`fs.resolve("")`, "fs.resolve: path must not be empty"},
		{`fs.resolve()`, "fs.resolve"},
		{`fs.resolve(1)`, "fs.resolve"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			t.Parallel()
			_, err := New().Execute(context.Background(), script.Spec{WorkingDirectory: t.TempDir(), Source: tt.source})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestFileSystemResolveHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().Execute(ctx, script.Spec{WorkingDirectory: t.TempDir(), Source: `fs.resolve("x")`})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFileSystemTildePaths(t *testing.T) {
	home := setTestHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, "x.txt"), []byte("hello"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, "tool.star"), []byte(""), 0o600))
	result, err := New().Execute(context.Background(), script.Spec{
		WorkingDirectory: t.TempDir(), Source: `
output = [fs.exists("~"), fs.exists("~/x.txt"), fs.exists("~/missing"), fs.read_file("~/x.txt"), fs.stat("~/x.txt").size, fs.glob("~/*.star") == [env["HOME_STAR"]]]
`,
		Env: map[string]string{"HOME_STAR": filepath.ToSlash(filepath.Join(home, "tool.star"))},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[true,true,false,"hello",5,true]`, result.Value)
}
