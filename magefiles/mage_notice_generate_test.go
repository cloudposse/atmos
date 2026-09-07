//go:build mage

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNoticeGeneratePropagatesRepoRootError covers the branch of Generate
// that's safe to unit test without a real tools/noticegen tree: repo-root
// resolution failing before any subprocess is spawned.
func TestNoticeGeneratePropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())
	err := Notice{}.Generate()
	require.ErrorIs(t, err, errMageRepoRootNotFound)
}

// TestNoticeGeneratePropagatesRunError covers the `sh.RunWithV` failure
// branch: a resolvable repo root but a tools/noticegen dir that doesn't
// exist, so the real `go run -C ...` invocation fails fast without needing
// this repo's actual noticegen tool (whose own tests cover its logic).
func TestNoticeGeneratePropagatesRunError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	t.Chdir(root)

	err := Notice{}.Generate()
	require.Error(t, err)
}

// TestNoticeGenerateSuccess covers the happy path (`return nil`): a real
// `go run` of a trivial, go-licenses-independent stand-in program under
// tools/noticegen that writes a fixed marker to its second argument (the
// NOTICE path Generate passes it) and exits 0. It verifies Generate's own
// orchestration (the toolDir and args it assembles) without depending on
// the real noticegen tool's own report-generation logic, which is
// unit-tested independently under tools/noticegen.
func TestNoticeGenerateSuccess(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
	toolDir := filepath.Join(root, "tools", "noticegen")
	require.NoError(t, os.MkdirAll(toolDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "go.mod"),
		[]byte("module github.com/cloudposse/atmos/tools/noticegen\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(toolDir, "main.go"),
		[]byte(`package main

import "os"

func main() {
	os.WriteFile(os.Args[2], []byte("stub notice"), 0o644)
}
`), 0o644))
	t.Chdir(root)

	require.NoError(t, Notice{}.Generate())

	data, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	require.NoError(t, err)
	require.Equal(t, "stub notice", string(data))
}
