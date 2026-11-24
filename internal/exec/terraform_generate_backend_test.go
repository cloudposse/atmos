package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteTerraformGenerateBackendCmd_Deprecated tests the deprecated command returns an error.

// TestWriteBackendConfigFile tests the writeBackendConfigFile function across dry-run, normal write, and workdir cases.

// TestExecuteGenerateBackend_ProcessStacksFails tests that ExecuteGenerateBackend returns an error
// when ProcessStacks fails due to missing stack config files.
func TestExecuteGenerateBackend_ProcessStacksFails(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	opts := &GenerateBackendOptions{
		Component: "vpc",
		Stack:     "dev",
	}

	err := ExecuteGenerateBackend(opts, atmosConfig)
	assert.Error(t, err, "should fail when stack config is not set up")
}
