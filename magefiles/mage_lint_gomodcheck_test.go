//go:build mage

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLintGoModCheckPropagatesRepoRootError covers the branch of GoModCheck
// that's safe to unit test without a real tools/gomodcheck tree: repo-root
// resolution failing before any subprocess is spawned.
func TestLintGoModCheckPropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())
	err := Lint{}.GoModCheck()
	require.ErrorIs(t, err, errMageRepoRootNotFound)
}

// TestLintGoModCheckPropagatesRunError covers the `sh.RunWithV` failure
// branch: a resolvable repo root but a tools/gomodcheck dir that doesn't
// exist, so the real `go run -C ...` invocation fails fast without needing
// this repo's actual gomodcheck tool (whose happy path has its own test
// suite and is exercised for real by CI's `go tool mage lint:goModCheck`).
func TestLintGoModCheckPropagatesRunError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	t.Chdir(root)

	err := Lint{}.GoModCheck()
	require.Error(t, err)
}
