//go:build !windows

package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEnsureTerraformComponentExists_DirectoryCheckError tests the error propagation
// when checkDirectoryExists returns a real filesystem error (e.g., permission denied).
func TestEnsureTerraformComponentExists_DirectoryCheckError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()

	// Create the components/terraform directory but make it inaccessible.
	componentBase := filepath.Join(tempDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(filepath.Join(componentBase, "vpc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentBase, "vpc", "main.tf"), []byte("# vpc\n"), 0o644))

	// Remove permissions on the component base directory to trigger a real filesystem error.
	require.NoError(t, os.Chmod(componentBase, 0o000))
	t.Cleanup(func() {
		os.Chmod(componentBase, 0o755)
	})

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := ensureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "should propagate filesystem error from checkDirectoryExists")
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
}

// TestCheckDirectoryExists_PermissionError tests the real filesystem error branch.
func TestCheckDirectoryExists_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()
	restrictedDir := filepath.Join(tempDir, "restricted")
	require.NoError(t, os.MkdirAll(restrictedDir, 0o755))

	targetDir := filepath.Join(restrictedDir, "inner")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	// Remove read+execute permission on parent to cause a real filesystem error.
	require.NoError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() {
		// Restore permissions so cleanup can succeed.
		os.Chmod(restrictedDir, 0o755)
	})

	// Attempting to stat the inner directory should trigger a permission error.
	exists, err := checkDirectoryExists(targetDir)
	assert.Error(t, err, "should return error for permission denied")
	assert.False(t, exists)
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
}
