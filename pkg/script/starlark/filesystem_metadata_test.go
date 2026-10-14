package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/automation"
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
