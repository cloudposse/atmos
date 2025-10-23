package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteTerraformGenerateVarfiles tests the ExecuteTerraformGenerateVarfiles function.
func TestExecuteTerraformGenerateVarfiles(t *testing.T) {
	t.Run("generates varfiles in JSON format", func(t *testing.T) {
		// Create a temporary directory for terraform components
		tempDir := t.TempDir()
		componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
		err := os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Create atmos config
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				BasePath:    "stacks",
				NamePattern: "{tenant}-{environment}-{stage}",
			},
		}

		// Set ATMOS_LOGS_LEVEL to suppress debug output
		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Call the function with empty stacks map (no actual stacks to process)
		err = ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{}, []string{})

		// Should succeed even with no stacks
		assert.NoError(t, err)
	})

	t.Run("validates format parameter", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Test with valid formats - should not error
		err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{}, []string{})
		assert.NoError(t, err)

		err = ExecuteTerraformGenerateVarfiles(atmosConfig, "", "yaml", []string{}, []string{})
		assert.NoError(t, err)

		err = ExecuteTerraformGenerateVarfiles(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})

	t.Run("handles file template with context tokens", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// File template with context tokens
		fileTemplate := filepath.Join(tempDir, "varfiles", "{tenant}", "{environment}", "{component}.tfvars.json")

		err := ExecuteTerraformGenerateVarfiles(atmosConfig, fileTemplate, "json", []string{}, []string{})
		assert.NoError(t, err)
	})

	t.Run("handles specific stacks filter", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Pass specific stacks to filter
		stacks := []string{"dev", "prod"}
		err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", stacks, []string{})
		assert.NoError(t, err)
	})

	t.Run("handles specific components filter", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Pass specific components to filter
		components := []string{"vpc", "eks"}
		err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{}, components)
		assert.NoError(t, err)
	})
}

// TestGenerateVarfilesWithMultipleFormats tests varfile generation with different formats.
func TestGenerateVarfilesWithMultipleFormats(t *testing.T) {
	testCases := []struct {
		name   string
		format string
	}{
		{
			name:   "JSON format",
			format: "json",
		},
		{
			name:   "YAML format",
			format: "yaml",
		},
		{
			name:   "HCL format",
			format: "hcl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tempDir,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			t.Setenv("ATMOS_LOGS_LEVEL", "Error")

			err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", tc.format, []string{}, []string{})
			assert.NoError(t, err)
		})
	}
}
