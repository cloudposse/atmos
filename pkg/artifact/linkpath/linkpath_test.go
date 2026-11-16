package linkpath_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		name    string
		path    string
		want    string
		wantErr error
	}{
		{"plain file", filepath.Join(root, "tree", "real", "file.txt"), filepath.Join(root, "tree", "real", "file.txt"), nil},
		{"link to directory", filepath.Join(root, "tree", "to-real"), filepath.Join(root, "tree", "real"), nil},
		{"dangling tail stays lexical under the link target", filepath.Join(root, "tree", "dangling"), filepath.Join(root, "tree", "real", "missing", "deeper"), nil},
		{"dangling tail walking out through a link", filepath.Join(root, "tree", "escapes"), filepath.Join(filepath.Dir(root), "outside", "missing"), nil},
		{"missing path is lexical", filepath.Join(root, "tree", "absent", "..", "other"), filepath.Join(root, "tree", "other"), nil},
		{"missing tail returns to physical resolution", filepath.Join(root, "tree", "missing-return"), filepath.Join(filepath.Dir(root), "outside"), nil},
		{"loop", filepath.Join(root, "tree", "loop-a"), "", errUtils.ErrSymlinkLoop},
		{"through a regular file", filepath.Join(root, "tree", "real", "file.txt", "child"), "", errUtils.ErrPathThroughNonDirectory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := linkpath.Resolve(tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A parent component after a symlink applies to the link's physical target, not to the link's
// directory: "jump/../secret" is a sibling of the directory jump points into.
func TestResolve_ParentComponentFollowsTheLinkPhysically(t *testing.T) {
	root := realTemp(t)
	fixtures.WriteFile(t, root, "elsewhere/deep/keep.txt", "x", 0o644)
	if !fixtures.Symlink(t, root, "tree/jump", "../elsewhere/deep") {
		t.Skip("this platform cannot create symlinks")
	}
	// Written as a joined string so the path keeps its ".." component.
	input := filepath.Join(root, "tree", "jump") + string(filepath.Separator) + ".." + string(filepath.Separator) + "secret"

	got, err := linkpath.Resolve(input)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "elsewhere", "secret"), got)
}

func TestResolve_AbsoluteLinkTarget(t *testing.T) {
	root := realTemp(t)
	target := filepath.Join(root, "target")
	require.NoError(t, os.MkdirAll(target, 0o755))
	link := filepath.Join(root, "abs")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable here: %v", err)
	}
	got, err := linkpath.Resolve(filepath.Join(link, "missing"))
	require.NoError(t, err)
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

func TestEscapes(t *testing.T) {
	tests := []struct {
		name  string
		links map[string]string
		from  string
		want  bool
	}{
		{"link to a sibling stays inside", map[string]string{"a": "b"}, "a", false},
		{"link above the root escapes", map[string]string{"a": ".."}, "a", true},
		{"link to a missing path below a link to the root escapes through it", map[string]string{"d": ".", "L": "d/../outside/missing"}, "L", true},
		{"the same link inside the root when the directory link goes deeper", map[string]string{"d": "sub", "L": "d/../outside/missing"}, "L", false},
		{"chained links that climb out escape", map[string]string{"a/b": "..", "c": "a/b/../.."}, "c", true},
		{"chained links that stay inside", map[string]string{"a/b": "..", "c": "a/b/x"}, "c", false},
		{"a link inside a linked directory is read through the link", map[string]string{"d": "sub", "sub/L": "../.."}, "d/L", true},
		{"a dangling link beyond the root escapes", map[string]string{"a": "x/../../y"}, "a", true},
		{"a loop does not escape and terminates", map[string]string{"p": "q", "q": "p"}, "p", false},
		{"a plain name is not a link", map[string]string{}, "plain", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, linkpath.Escapes(tc.links, tc.from))
		})
	}
}
