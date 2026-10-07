//go:build mage

package mageutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMageRepoRoot(t *testing.T) {
	t.Run("finds root from nested subdir", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(RootModuleDecl+"\n\ngo 1.26\n"), 0o644))
		nested := filepath.Join(root, "a", "b", "c")
		require.NoError(t, os.MkdirAll(nested, 0o755))

		t.Chdir(nested)
		got, err := RepoRoot()
		require.NoError(t, err)
		assert.Equal(t, root, got)
	})

	t.Run("does not match a go.mod for a different module", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/other\n\ngo 1.26\n"), 0o644))

		t.Chdir(root)
		_, err := RepoRoot()
		require.ErrorIs(t, err, ErrRepoRootNotFound)
	})

	t.Run("does not stop at a nested module that is a prefix of the root module", func(t *testing.T) {
		// tools/lintroller's own go.mod ("module
		// github.com/cloudposse/atmos/tools/lintroller") is textually a
		// strings.HasPrefix match against RootModuleDecl ("module
		// github.com/cloudposse/atmos"). RepoRoot must require an exact
		// first-line match so it walks past this nested module up to the
		// real root instead of stopping here.
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(RootModuleDecl+"\n\ngo 1.26\n"), 0o644))
		nested := filepath.Join(root, "tools", "lintroller")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(nested, "go.mod"), []byte(RootModuleDecl+"/tools/lintroller\n\ngo 1.24\n"), 0o644))

		t.Chdir(nested)
		got, err := RepoRoot()
		require.NoError(t, err)
		assert.Equal(t, root, got)
	})
}

func TestMageGoVersion(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		got, err := GoVersion()
		require.NoError(t, err)
		assert.True(t, len(got) > 2 && got[:2] == "go", "expected version to start with \"go\", got %q", got)
	})

	t.Run("go binary not on PATH", func(t *testing.T) {
		t.Setenv("PATH", "")
		_, err := GoVersion()
		require.Error(t, err)
	})
}

func TestIsGoSourceFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"foo.go", true},
		{".go", true},
		{"foo.txt", false},
		{"", false},
		{"foo.go.bak", false},
		{"FOO.GO", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, IsGoSourceFile(tc.name), "IsGoSourceFile(%q)", tc.name)
	}
}

func TestIsGoModuleFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"foo.go", true},
		{"go.mod", true},
		{"go.sum", true},
		{"go.sum.bak", false},
		{"README.md", false},
		{"", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, IsGoModuleFile(tc.name), "IsGoModuleFile(%q)", tc.name)
	}
}

func TestDirHasFileNewerThan(t *testing.T) {
	goMatch := func(name string) bool { return name == "match.go" }

	t.Run("matching file newer than cutoff", func(t *testing.T) {
		dir := t.TempDir()
		cutoff := time.Now()
		path := filepath.Join(dir, "match.go")
		require.NoError(t, os.WriteFile(path, []byte("package x"), 0o644))
		require.NoError(t, os.Chtimes(path, time.Time{}, cutoff.Add(time.Hour)))

		got, err := DirHasFileNewerThan(dir, cutoff, goMatch)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("matching file older than cutoff", func(t *testing.T) {
		dir := t.TempDir()
		cutoff := time.Now()
		path := filepath.Join(dir, "match.go")
		require.NoError(t, os.WriteFile(path, []byte("package x"), 0o644))
		require.NoError(t, os.Chtimes(path, time.Time{}, cutoff.Add(-time.Hour)))

		got, err := DirHasFileNewerThan(dir, cutoff, goMatch)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("non-matching newer file is ignored", func(t *testing.T) {
		dir := t.TempDir()
		cutoff := time.Now()
		path := filepath.Join(dir, "other.txt")
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
		require.NoError(t, os.Chtimes(path, time.Time{}, cutoff.Add(time.Hour)))

		got, err := DirHasFileNewerThan(dir, cutoff, goMatch)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		got, err := DirHasFileNewerThan(dir, time.Now(), goMatch)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("match in nested subdir", func(t *testing.T) {
		dir := t.TempDir()
		cutoff := time.Now()
		nested := filepath.Join(dir, "nested")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		path := filepath.Join(nested, "match.go")
		require.NoError(t, os.WriteFile(path, []byte("package x"), 0o644))
		require.NoError(t, os.Chtimes(path, time.Time{}, cutoff.Add(time.Hour)))

		got, err := DirHasFileNewerThan(dir, cutoff, goMatch)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("dir does not exist", func(t *testing.T) {
		_, err := DirHasFileNewerThan(filepath.Join(t.TempDir(), "missing"), time.Now(), goMatch)
		require.Error(t, err)
	})
}
