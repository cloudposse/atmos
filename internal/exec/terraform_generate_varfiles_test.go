package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteTerraformGenerateVarfiles_ParameterHandling tests parameter acceptance.
func TestExecuteTerraformGenerateVarfiles_ParameterHandling(t *testing.T) {
	t.Run("accepts valid formats without error", func(t *testing.T) {
		tempDir := t.TempDir()

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

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Test valid formats - should not error (even with no stacks).
		validFormats := []string{"json", "yaml", "hcl"}
		for _, format := range validFormats {
			err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", format, []string{}, []string{})
			assert.NoError(t, err, "format %s should be valid", format)
		}
	})

	t.Run("accepts component and stack filters", func(t *testing.T) {
		tempDir := t.TempDir()

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

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Test with component filter.
		err := ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{}, []string{"vpc", "eks"})
		assert.NoError(t, err, "should accept component filters")

		// Test with stack filter.
		err = ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{"dev", "prod"}, []string{})
		assert.NoError(t, err, "should accept stack filters")

		// Test with both filters.
		err = ExecuteTerraformGenerateVarfiles(atmosConfig, "", "json", []string{"dev"}, []string{"vpc"})
		assert.NoError(t, err, "should accept both component and stack filters")
	})

	t.Run("accepts file template parameter", func(t *testing.T) {
		tempDir := t.TempDir()

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

		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Test with file template containing context tokens.
		fileTemplate := "/tmp/varfiles/{tenant}/{environment}/{component}.tfvars.json"
		err := ExecuteTerraformGenerateVarfiles(atmosConfig, fileTemplate, "json", []string{}, []string{})
		assert.NoError(t, err, "should accept file template with tokens")
	})
}
