//go:build !windows

package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
)

// TestGoGetterAdapter_NestedImportBasePathResolveError covers the branch where a remote
// file's nested-import base_path cannot be resolved. The downloaded parent declares a
// bare base_path resolving under an unreadable git-root directory (chmod 000), so
// ResolveConfigImportBasePath returns a permission-denied stat error that the adapter
// wraps as ErrProcessNestedImports. Unix-only; skipped as root.
func TestGoGetterAdapter_NestedImportBasePathResolveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
	config.ResetImportAdapterRegistry()
	config.SetDefaultAdapter(&LocalAdapter{})

	gitRoot := t.TempDir()
	t.Setenv("TEST_GIT_ROOT", gitRoot)
	locked := filepath.Join(gitRoot, "locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	tempDir := t.TempDir()
	parent := filepath.Join(tempDir, "parent.yaml")
	require.NoError(t, os.WriteFile(parent, []byte("base_path: locked/child\nimport:\n  - nested.yaml\n"), 0o644))

	adapter := &GoGetterAdapter{}
	_, err := adapter.Resolve(context.Background(), "file://"+parent, tempDir, tempDir, 1, 10, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProcessNestedImports)
}
