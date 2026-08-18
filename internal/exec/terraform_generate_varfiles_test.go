package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestExecuteTerraformGenerateVarfiles_DeferredMergeDataLoss is a regression test for #2888:
// a deferred YAML function (!labels) merged against a concrete override at the same `vars` path
// must deep-merge, not silently lose the function's contribution. ExecuteTerraformGenerateVarfiles
// builds its component config directly from the FindStacksMap cache rather than going through the
// same describe/plan pipeline (internal/exec/utils.go's ProcessStacks) that resolves deferred
// merges, so this exercises a separate call site of the same underlying bug.
func TestExecuteTerraformGenerateVarfiles_DeferredMergeDataLoss(t *testing.T) {
	tempDir := t.TempDir()

	stacksDir := filepath.Join(tempDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# vpc component\n"), 0o644))

	// base-labels is an abstract base component whose `vars.tags` is a deferred !labels function
	// (derived from its own metadata.labels). vpc inherits from it via `component:` and layers a
	// conflicting literal map at the same `vars.tags` path — the two must deep-merge (org/region
	// from !labels, team from the override), not have one silently discard the other.
	stackContent := `
vars:
  stage: dev
components:
  terraform:
    base-labels:
      metadata:
        type: abstract
        labels:
          org: acme
          region: us-east-1
      vars:
        tags: !labels
    vpc:
      component: base-labels
      backend:
        bucket: test-bucket
        key: terraform.tfstate
      backend_type: s3
      vars:
        name: test-vpc
        tags:
          team: platform
`
	stackFile := filepath.Join(stacksDir, "dev.yaml")
	require.NoError(t, os.WriteFile(stackFile, []byte(stackContent), 0o644))

	outputFile := filepath.Join(tempDir, "vpc.tfvars.json")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{stage}",
		},
		StacksBaseAbsolutePath:        stacksDir,
		TerraformDirAbsolutePath:      filepath.Join(tempDir, "components", "terraform"),
		IncludeStackAbsolutePaths:     []string{stacksDir},
		StackConfigFilesAbsolutePaths: []string{stackFile},
	}

	err := ExecuteTerraformGenerateVarfiles(atmosConfig, outputFile, "json", []string{}, []string{"vpc"})
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	var written map[string]any
	require.NoError(t, json.Unmarshal(content, &written))

	tags, ok := written["tags"].(map[string]any)
	require.True(t, ok, "vars.tags must be a map in the generated varfile, got: %v", written["tags"])
	assert.Equal(t, map[string]any{
		"org":    "acme",
		"region": "us-east-1",
		"team":   "platform",
	}, tags, "deferred !labels output must deep-merge with the component's own vars.tags override, not be silently dropped")
}
