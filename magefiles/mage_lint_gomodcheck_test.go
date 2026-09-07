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

// TestLintGoModCheckSuccess covers the happy path (`return nil`): a real
// `go run` of a trivial, dependency-free stand-in program under
// tools/gomodcheck that exits 0. It verifies GoModCheck's own orchestration
// (the toolDir and goModPath args it assembles) and that it returns nil on
// success, without depending on the real gomodcheck tool's own validation
// logic, which is unit-tested independently under tools/gomodcheck.
func TestLintGoModCheckSuccess(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	toolDir := filepath.Join(root, "tools", "gomodcheck")
	require.NoError(t, os.MkdirAll(toolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "go.mod"),
		[]byte("module github.com/cloudposse/atmos/tools/gomodcheck\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644))
	t.Chdir(root)

	require.NoError(t, Lint{}.GoModCheck())
}
