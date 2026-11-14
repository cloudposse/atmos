package linkpath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/linkpath"
)

// realTemp returns a temporary directory with symlinks in its own path already resolved.
func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestResolve(t *testing.T) {
	root := realTemp(t)
	fixtures.WriteFile(t, root, "tree/real/file.txt", "x", 0o644)
	if !fixtures.Symlink(t, root, "tree/up", "..") {
		t.Skip("this platform cannot create symlinks")
	}
	require.True(t, fixtures.Symlink(t, root, "tree/to-real", "real"))
	require.True(t, fixtures.Symlink(t, root, "tree/dangling", "real/missing/deeper"))
	require.True(t, fixtures.Symlink(t, root, "tree/escapes", "up/../outside/missing"))
	require.True(t, fixtures.Symlink(t, root, "tree/missing-return", "absent/../up/../outside"))
	require.True(t, fixtures.Symlink(t, root, "tree/loop-a", "loop-b"))
	require.True(t, fixtures.Symlink(t, root, "tree/loop-b", "loop-a"))

	tests := []struct {
		name      string
		path      string
		want      string
		reachable bool
	}{
		{"plain file", filepath.Join(root, "tree", "real", "file.txt"), filepath.Join(root, "tree", "real", "file.txt"), true},
		{"link to directory", filepath.Join(root, "tree", "to-real"), filepath.Join(root, "tree", "real"), true},
		{"dangling tail stays lexical under the link target", filepath.Join(root, "tree", "dangling"), filepath.Join(root, "tree", "real", "missing", "deeper"), true},
		{"dangling tail walking out through a link", filepath.Join(root, "tree", "escapes"), filepath.Join(filepath.Dir(root), "outside", "missing"), true},
		{"missing path is lexical", filepath.Join(root, "tree", "absent", "..", "other"), filepath.Join(root, "tree", "other"), true},
		{"missing tail returns to physical resolution", filepath.Join(root, "tree", "missing-return"), filepath.Join(filepath.Dir(root), "outside"), true},
		{"loop", filepath.Join(root, "tree", "loop-a"), "", false},
		{"through a regular file", filepath.Join(root, "tree", "real", "file.txt", "child"), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reachable := linkpath.Resolve(tt.path)
			assert.Equal(t, tt.reachable, reachable)
			if tt.reachable {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestResolve_AbsoluteLinkTarget(t *testing.T) {
	root := realTemp(t)
	target := filepath.Join(root, "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	link := filepath.Join(root, "abs")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	got, reachable := linkpath.Resolve(filepath.Join(link, "missing"))
	assert.True(t, reachable)
	assert.Equal(t, filepath.Join(target, "missing"), got)
}

func TestWithin(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "base", "root")
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"root itself", root, true},
		{"descendant", filepath.Join(root, "a", "b"), true},
		{"sibling with shared prefix", root + "-other", false},
		{"parent", filepath.Dir(root), false},
		{"unrelated", filepath.Join(string(filepath.Separator), "elsewhere"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, linkpath.Within(root, tt.target))
		})
	}
}
