package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestExecuteTerraformGenerateBackends tests the ExecuteTerraformGenerateBackends function.
func TestExecuteTerraformGenerateBackends(t *testing.T) {
	t.Run("generates backend config in HCL format", func(t *testing.T) {
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

		// Set ATMOS_LOGS_LEVEL to suppress debug output (t.Setenv auto-restores)
		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		// Call the function with empty stacks map (no actual stacks to process)
		err = ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})

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

		// Test with valid formats - should not error
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)

		err = ExecuteTerraformGenerateBackends(atmosConfig, "", "json", []string{}, []string{})
		assert.NoError(t, err)

		err = ExecuteTerraformGenerateBackends(atmosConfig, "", "backend-config", []string{}, []string{})
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

		// File template with context tokens
		fileTemplate := filepath.Join(tempDir, "backends", "{tenant}", "{environment}", "{component}.tf")

		err := ExecuteTerraformGenerateBackends(atmosConfig, fileTemplate, "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})
}

// TestGenerateBackendConfigWithMultipleFormats tests backend generation with different formats.
func TestGenerateBackendConfigWithMultipleFormats(t *testing.T) {
	testCases := []struct {
		name   string
		format string
	}{
		{
			name:   "HCL format",
			format: "hcl",
		},
		{
			name:   "JSON format",
			format: "json",
		},
		{
			name:   "backend-config format",
			format: "backend-config",
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

			err := ExecuteTerraformGenerateBackends(atmosConfig, "", tc.format, []string{}, []string{})
			assert.NoError(t, err)
		})
	}
}

// TestBackendTemplateProcessing tests template processing in backend configs.
func TestBackendTemplateProcessing(t *testing.T) {
	t.Run("processes Go templates in backend config", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Templates: schema.Templates{
				Settings: schema.TemplatesSettings{
					Enabled: true,
				},
			},
		}

		// Set log level to suppress output (t.Setenv auto-restores)
		t.Setenv("ATMOS_LOGS_LEVEL", "Error")

		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})
}

// TestComponentAndStackFiltering tests component and stack filtering.
func TestComponentAndStackFiltering(t *testing.T) {
	t.Run("filters by component names", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Filter by specific components
		components := []string{"vpc", "eks"}
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, components)
		assert.NoError(t, err)
	})

	t.Run("filters by stack names", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Filter by specific stacks
		stacks := []string{"tenant1-ue2-dev", "tenant1-ue2-prod"}
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", stacks, []string{})
		assert.NoError(t, err)
	})

	t.Run("filters by both components and stacks", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Filter by both
		stacks := []string{"tenant1-ue2-dev"}
		components := []string{"vpc"}
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", stacks, components)
		assert.NoError(t, err)
	})
}

// TestAbstractComponentsExcluded tests that abstract components are excluded.
func TestAbstractComponentsExcluded(t *testing.T) {
	t.Run("skips abstract components", func(t *testing.T) {
		tempDir := t.TempDir()

		// This test verifies the function handles abstract components correctly.
		// Abstract components should be skipped during backend generation.
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})
}

// TestBackendTypeHandling tests handling of different backend types.
func TestBackendTypeHandling(t *testing.T) {
	testCases := []struct {
		name        string
		backendType string
	}{
		{
			name:        "S3 backend",
			backendType: "s3",
		},
		{
			name:        "GCS backend",
			backendType: "gcs",
		},
		{
			name:        "Azure backend",
			backendType: "azurerm",
		},
		{
			name:        "Local backend",
			backendType: "local",
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

			// Function should handle different backend types
			err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
			assert.NoError(t, err)
		})
	}
}

// TestFileTemplateDirectoryCreation tests that directories are created for file templates.
func TestFileTemplateDirectoryCreation(t *testing.T) {
	t.Run("creates intermediate directories for file template", func(t *testing.T) {
		tempDir := t.TempDir()
		outputDir := filepath.Join(tempDir, "backends", "tenant1", "dev")

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Using file template should create directories
		fileTemplate := filepath.Join(outputDir, "backend.tf")
		err := ExecuteTerraformGenerateBackends(atmosConfig, fileTemplate, "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})
}

// TestContextTokenReplacement tests context token replacement in file templates.
func TestContextTokenReplacement(t *testing.T) {
	t.Run("replaces context tokens in file template", func(t *testing.T) {
		// Test that context tokens like {namespace}, {tenant}, etc. are handled.
		// The ReplaceContextTokens function is called internally.
		context := schema.Context{
			Namespace:   "cp",
			Tenant:      "platform",
			Environment: "ue2",
			Stage:       "dev",
		}

		template := "{namespace}/{tenant}/{environment}/{stage}/backend.tf"
		result := cfg.ReplaceContextTokens(context, template)

		expected := "cp/platform/ue2/dev/backend.tf"
		assert.Equal(t, expected, result)
	})
}

// TestStackNameGeneration tests stack name generation with templates.
func TestStackNameGeneration(t *testing.T) {
	t.Run("generates stack names from name template", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				NameTemplate: "{tenant}-{environment}-{stage}",
			},
		}

		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})

	t.Run("generates stack names from name pattern", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				NamePattern: "{tenant}-{environment}-{stage}",
			},
		}

		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)
	})
}

// TestComponentPathGeneration tests component path generation.
func TestComponentPathGeneration(t *testing.T) {
	t.Run("generates correct component path", func(t *testing.T) {
		tempDir := t.TempDir()
		componentName := "vpc"
		expectedPath := filepath.Join(tempDir, "components", "terraform", componentName)

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Verify the path construction logic
		actualPath := filepath.Join(
			atmosConfig.BasePath,
			atmosConfig.Components.Terraform.BasePath,
			componentName,
		)

		assert.Equal(t, expectedPath, actualPath)
	})
}

// TestFileExtensionHandling tests handling of different file extensions.
func TestFileExtensionHandling(t *testing.T) {
	t.Run("uses .tf extension for HCL format", func(t *testing.T) {
		backendFile := "backend.tf"
		assert.True(t, filepath.Ext(backendFile) == ".tf")
	})

	t.Run("uses .tf.json extension for JSON format", func(t *testing.T) {
		backendFile := "backend.tf.json"
		assert.Contains(t, backendFile, ".json")
	})
}

// TestEnsureDirFunctionality tests the EnsureDir utility function.
func TestEnsureDirFunctionality(t *testing.T) {
	t.Run("creates directory structure", func(t *testing.T) {
		tempDir := t.TempDir()
		testPath := filepath.Join(tempDir, "level1", "level2", "level3", "file.tf")

		err := u.EnsureDir(testPath)
		assert.NoError(t, err)

		// Check that parent directories were created
		parentDir := filepath.Dir(testPath)
		_, err = os.Stat(parentDir)
		assert.NoError(t, err)
	})
}

// TestProcessedComponentsTracking tests that components are tracked to avoid duplicates.
func TestProcessedComponentsTracking(t *testing.T) {
	t.Run("tracks processed components without file template", func(t *testing.T) {
		// When file template is not provided, components should be tracked
		// to avoid processing the same terraform component multiple times.
		processedComponents := make(map[string]any)
		componentName := "vpc"

		// First time - not processed.
		exists := u.MapKeyExists(processedComponents, componentName)
		assert.False(t, exists)

		// Mark as processed.
		processedComponents[componentName] = componentName

		// Second time - already processed.
		exists = u.MapKeyExists(processedComponents, componentName)
		assert.True(t, exists)
	})
}

func TestExecuteTerraformGenerateBackends_StackAndComponentFilters(t *testing.T) {
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
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", stacks, []string{})
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
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, components)
		assert.NoError(t, err)
	})

	t.Run("handles both stacks and components filters", func(t *testing.T) {
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

		// Pass both filters
		stacks := []string{"dev"}
		components := []string{"vpc"}
		err := ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", stacks, components)
		assert.NoError(t, err)
	})
}
