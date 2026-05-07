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

// TestEnsureTerraformComponentExists_DirectoryCheckError tests that
// non-ENOENT stat failures (e.g. EACCES) on the component path are propagated
// from the orchestrator (via component.componentDirExists) and wrapped with
// ErrInvalidTerraformComponent at the executor boundary.
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
	assert.Error(t, err, "should propagate filesystem error from the orchestrator")
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
}
