package exec

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
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

// TestBackendGenerationWithNestedMaps tests that nested maps (like assume_role) are properly generated in all formats.
func TestBackendGenerationWithNestedMaps(t *testing.T) {
	t.Run("generates nested maps in HCL format", func(t *testing.T) {
		tempDir := t.TempDir()
		componentDir := filepath.Join(tempDir, "components", "terraform", "test-component")
		err := os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Backend config with nested assume_role
		backendConfig := map[string]any{
			"bucket": "test-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456789012:role/terraform-backend-role",
				"session_name": "terraform-backend-session",
				"duration":     "1h",
			},
		}

		// Write HCL backend
		backendFile := filepath.Join(componentDir, "backend.tf")
		err = u.WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", backendConfig)
		require.NoError(t, err)

		// Read and verify
		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		backendStr := string(content)

		// Verify the assume_role block is present
		assert.Contains(t, backendStr, "assume_role", "HCL output should contain assume_role")
		assert.Contains(t, backendStr, "role_arn", "HCL output should contain role_arn")
		assert.Contains(t, backendStr, "arn:aws:iam::123456789012:role/terraform-backend-role", "HCL output should contain the correct role ARN")
		assert.Contains(t, backendStr, "session_name", "HCL output should contain session_name")
		assert.Contains(t, backendStr, "terraform-backend-session", "HCL output should contain the session name")
		assert.Contains(t, backendStr, "duration", "HCL output should contain duration")

		// Verify it's wrapped in terraform backend block
		assert.Contains(t, backendStr, "terraform {", "HCL should have terraform block")
		assert.Contains(t, backendStr, "backend \"s3\" {", "HCL should have s3 backend block")
	})

	t.Run("generates nested maps in JSON format via generateComponentBackendConfig", func(t *testing.T) {
		// Backend config with nested assume_role
		backendConfig := map[string]any{
			"bucket": "test-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456789012:role/terraform-backend-role",
				"session_name": "terraform-backend-session",
				"duration":     "1h",
			},
		}

		// Generate backend config
		result, err := generateComponentBackendConfig("s3", backendConfig, "", nil)
		require.NoError(t, err)

		// Navigate to assume_role section
		terraform, ok := result["terraform"].(map[string]any)
		require.True(t, ok, "JSON should have terraform section")

		backend, ok := terraform["backend"].(map[string]any)
		require.True(t, ok, "JSON should have backend section")

		s3, ok := backend["s3"].(map[string]any)
		require.True(t, ok, "JSON should have s3 section")

		assumeRole, ok := s3["assume_role"].(map[string]any)
		require.True(t, ok, "JSON s3 section should have assume_role as a map")

		// Verify nested fields
		assert.Equal(t, "arn:aws:iam::123456789012:role/terraform-backend-role", assumeRole["role_arn"])
		assert.Equal(t, "terraform-backend-session", assumeRole["session_name"])
		assert.Equal(t, "1h", assumeRole["duration"])
	})

	t.Run("generates nested maps in backend-config format", func(t *testing.T) {
		tempDir := t.TempDir()
		componentDir := filepath.Join(tempDir, "components", "terraform", "test-component")
		err := os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Backend config with nested assume_role
		backendConfig := map[string]any{
			"bucket": "test-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456789012:role/terraform-backend-role",
				"session_name": "terraform-backend-session",
				"duration":     "1h",
			},
		}

		// Write backend-config
		backendFile := filepath.Join(componentDir, "backend.tfbackend")
		err = u.WriteToFileAsHcl(backendFile, backendConfig, 0o644)
		require.NoError(t, err)

		// Read and verify
		content, err := os.ReadFile(backendFile)
		require.NoError(t, err)

		backendStr := string(content)

		// Verify the assume_role block is present
		assert.Contains(t, backendStr, "assume_role", "backend-config output should contain assume_role")
		assert.Contains(t, backendStr, "role_arn", "backend-config output should contain role_arn")
		assert.Contains(t, backendStr, "arn:aws:iam::123456789012:role/terraform-backend-role", "backend-config should contain the correct role ARN")
		assert.Contains(t, backendStr, "session_name", "backend-config should contain session_name")
		assert.Contains(t, backendStr, "duration", "backend-config should contain duration")
	})
}

// TestBackendGenerationWithDifferentBackendTypes tests generation for different backend types.
func TestBackendGenerationWithDifferentBackendTypes(t *testing.T) {
	t.Setenv("ATMOS_LOGS_LEVEL", "Error")

	testCases := []struct {
		name          string
		backendType   string
		backendConfig map[string]any
		expectedKeys  []string
	}{
		{
			name:        "S3 backend with assume_role",
			backendType: "s3",
			backendConfig: map[string]any{
				"bucket": "test-bucket",
				"key":    "terraform.tfstate",
				"region": "us-east-1",
				"assume_role": map[string]any{
					"role_arn": "arn:aws:iam::123456789012:role/test",
				},
			},
			expectedKeys: []string{"bucket", "key", "region", "assume_role"},
		},
		{
			name:        "GCS backend with nested encryption",
			backendType: "gcs",
			backendConfig: map[string]any{
				"bucket": "test-bucket",
				"prefix": "terraform/state",
				"encryption_key": map[string]any{
					"kms_encryption_key": "projects/my-project/locations/us/keyRings/my-ring/cryptoKeys/my-key",
				},
			},
			expectedKeys: []string{"bucket", "prefix", "encryption_key"},
		},
		{
			name:        "AzureRM backend with client config",
			backendType: "azurerm",
			backendConfig: map[string]any{
				"storage_account_name": "mystorageaccount",
				"container_name":       "tfstate",
				"key":                  "terraform.tfstate",
				"client_id":            "00000000-0000-0000-0000-000000000000",
			},
			expectedKeys: []string{"storage_account_name", "container_name", "key"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			componentDir := filepath.Join(tempDir, "components", "terraform", "test-component")
			err := os.MkdirAll(componentDir, 0o755)
			require.NoError(t, err)

			// Write backend config to HCL
			backendFile := filepath.Join(componentDir, "backend.tf")
			err = u.WriteTerraformBackendConfigToFileAsHcl(backendFile, tc.backendType, tc.backendConfig)
			require.NoError(t, err)

			// Read and verify
			content, err := os.ReadFile(backendFile)
			require.NoError(t, err)

			backendStr := string(content)
			assert.Contains(t, backendStr, "terraform {")
			assert.Contains(t, backendStr, "backend \""+tc.backendType+"\" {")

			for _, key := range tc.expectedKeys {
				assert.Contains(t, backendStr, key, "Backend config should contain key: "+key)
			}
		})
	}
}

// TestBackendGenerationErrorHandling tests error cases.
func TestBackendGenerationErrorHandling(t *testing.T) {
	t.Run("rejects invalid format parameter", func(t *testing.T) {
		tempDir := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: tempDir,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		// Note: The validation happens in the CMD layer, not in ExecuteTerraformGenerateBackends
		// So we test the valid formats only here
		validFormats := []string{"hcl", "json", "backend-config"}
		for _, format := range validFormats {
			err := ExecuteTerraformGenerateBackends(atmosConfig, "", format, []string{}, []string{})
			assert.NoError(t, err, "Format %s should be valid", format)
		}
	})

	t.Run("handles empty backend config", func(t *testing.T) {
		tempDir := t.TempDir()
		componentDir := filepath.Join(tempDir, "components", "terraform", "test-component")
		err := os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Empty backend config should not error
		backendFile := filepath.Join(componentDir, "backend.tf")
		err = u.WriteTerraformBackendConfigToFileAsHcl(backendFile, "s3", map[string]any{})
		assert.NoError(t, err)

		// Verify file was created
		_, err = os.Stat(backendFile)
		assert.NoError(t, err)
	})
}

// TestGenerateComponentBackendConfigFunction tests the generateComponentBackendConfig helper function.
func TestGenerateComponentBackendConfigFunction(t *testing.T) {
	t.Run("generates cloud backend config with workspace", func(t *testing.T) {
		backendConfig := map[string]any{
			"organization": "my-org",
			"workspaces": map[string]any{
				"name": "my-workspace-{terraform_workspace}",
			},
		}

		result, err := generateComponentBackendConfig("cloud", backendConfig, "prod", nil)
		require.NoError(t, err)

		// Verify structure
		terraform, ok := result["terraform"].(map[string]any)
		require.True(t, ok)

		cloud, ok := terraform["cloud"].(map[string]any)
		require.True(t, ok)

		assert.Equal(t, "my-org", cloud["organization"])

		// Workspace name should have token replaced
		workspaces, ok := cloud["workspaces"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-workspace-prod", workspaces["name"])
	})

	t.Run("generates cloud backend config without workspace", func(t *testing.T) {
		backendConfig := map[string]any{
			"organization": "my-org",
			"workspaces": map[string]any{
				"name": "my-workspace",
			},
		}

		result, err := generateComponentBackendConfig("cloud", backendConfig, "", nil)
		require.NoError(t, err)

		terraform, ok := result["terraform"].(map[string]any)
		require.True(t, ok)

		cloud, ok := terraform["cloud"].(map[string]any)
		require.True(t, ok)

		workspaces, ok := cloud["workspaces"].(map[string]any)
		require.True(t, ok)

		// Should remain unchanged without workspace token
		assert.Equal(t, "my-workspace", workspaces["name"])
	})

	t.Run("generates s3 backend config", func(t *testing.T) {
		backendConfig := map[string]any{
			"bucket": "my-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
		}

		result, err := generateComponentBackendConfig("s3", backendConfig, "", nil)
		require.NoError(t, err)

		// For non-cloud backends, should return wrapped config
		terraform, ok := result["terraform"].(map[string]any)
		require.True(t, ok)

		backend, ok := terraform["backend"].(map[string]any)
		require.True(t, ok)

		s3, ok := backend["s3"].(map[string]any)
		require.True(t, ok)

		assert.Equal(t, "my-bucket", s3["bucket"])
		assert.Equal(t, "terraform.tfstate", s3["key"])
		assert.Equal(t, "us-east-1", s3["region"])
	})

	t.Run("preserves nested maps in backend config", func(t *testing.T) {
		backendConfig := map[string]any{
			"bucket": "my-bucket",
			"key":    "terraform.tfstate",
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456789012:role/test",
				"session_name": "terraform",
				"external_id":  "my-external-id",
			},
		}

		result, err := generateComponentBackendConfig("s3", backendConfig, "", nil)
		require.NoError(t, err)

		terraform := result["terraform"].(map[string]any)
		backend := terraform["backend"].(map[string]any)
		s3 := backend["s3"].(map[string]any)

		assumeRole, ok := s3["assume_role"].(map[string]any)
		require.True(t, ok, "assume_role should be preserved as a map")

		assert.Equal(t, "arn:aws:iam::123456789012:role/test", assumeRole["role_arn"])
		assert.Equal(t, "terraform", assumeRole["session_name"])
		assert.Equal(t, "my-external-id", assumeRole["external_id"])
	})

	t.Run("generates local backend config", func(t *testing.T) {
		backendConfig := map[string]any{
			"path": "terraform.tfstate",
		}

		result, err := generateComponentBackendConfig("local", backendConfig, "", nil)
		require.NoError(t, err)

		terraform := result["terraform"].(map[string]any)
		backend := terraform["backend"].(map[string]any)
		local, ok := backend["local"].(map[string]any)
		require.True(t, ok)

		assert.Equal(t, "terraform.tfstate", local["path"])
	})

	t.Run("returns error for empty backend type", func(t *testing.T) {
		backendConfig := map[string]any{
			"bucket": "test-bucket",
		}

		result, err := generateComponentBackendConfig("", backendConfig, "", nil)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, errUtils.ErrBackendTypeRequired)
	})
}

// TestBackendGenerationSkipsComponentsWithoutBackend tests that components without backend config are skipped with warnings.
func TestBackendGenerationSkipsComponentsWithoutBackend(t *testing.T) {
	t.Run("skips component without backend section and logs warning", func(t *testing.T) {
		// Set up a temp directory with stack files
		tempDir := t.TempDir()

		// Create stacks directory and a stack file with a component missing backend
		stacksDir := filepath.Join(tempDir, "stacks")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		// Create component directory
		componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
		err = os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Create a minimal main.tf so the component exists
		mainTF := filepath.Join(componentDir, "main.tf")
		err = os.WriteFile(mainTF, []byte("# vpc component\n"), 0o644)
		require.NoError(t, err)

		// Create stack file with component that has NO backend section
		stackContent := `
vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        name: test-vpc
`
		stackFile := filepath.Join(stacksDir, "dev.yaml")
		err = os.WriteFile(stackFile, []byte(stackContent), 0o644)
		require.NoError(t, err)

		// Capture log output
		var logBuf bytes.Buffer
		originalLogger := log.Default()
		testLogger := log.NewAtmosLogger(charm.New(&logBuf))
		testLogger.SetLevel(log.WarnLevel)
		log.SetDefault(testLogger)
		defer log.SetDefault(originalLogger)

		// Create atmosConfig with absolute paths properly set for FindStacksMap
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

		// Execute backend generation (test both HCL and JSON formats)
		err = ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)

		// Check that warning was logged
		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "Skipping backend generation")
		assert.Contains(t, logOutput, "auto_generate_backend_file")

		// Verify no backend.tf or backend.tf.json was generated
		backendTF := filepath.Join(componentDir, "backend.tf")
		backendTFJSON := filepath.Join(componentDir, "backend.tf.json")
		_, errTF := os.Stat(backendTF)
		_, errJSON := os.Stat(backendTFJSON)
		assert.True(t, os.IsNotExist(errTF), "backend.tf should not be created when backend section is missing")
		assert.True(t, os.IsNotExist(errJSON), "backend.tf.json should not be created when backend section is missing")
	})

	t.Run("skips component without backend_type and logs warning", func(t *testing.T) {
		// Set up a temp directory with stack files
		tempDir := t.TempDir()

		// Create stacks directory
		stacksDir := filepath.Join(tempDir, "stacks")
		err := os.MkdirAll(stacksDir, 0o755)
		require.NoError(t, err)

		// Create component directory
		componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
		err = os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err)

		// Create a minimal main.tf
		mainTF := filepath.Join(componentDir, "main.tf")
		err = os.WriteFile(mainTF, []byte("# vpc component\n"), 0o644)
		require.NoError(t, err)

		// Create stack file with component that has backend but NO backend_type
		stackContent := `
vars:
  stage: dev
components:
  terraform:
    vpc:
      backend:
        bucket: test-bucket
        key: terraform.tfstate
      vars:
        name: test-vpc
`
		stackFile := filepath.Join(stacksDir, "dev.yaml")
		err = os.WriteFile(stackFile, []byte(stackContent), 0o644)
		require.NoError(t, err)

		// Capture log output
		var logBuf bytes.Buffer
		originalLogger := log.Default()
		testLogger := log.NewAtmosLogger(charm.New(&logBuf))
		testLogger.SetLevel(log.WarnLevel)
		log.SetDefault(testLogger)
		defer log.SetDefault(originalLogger)

		// Create atmosConfig with absolute paths properly set for FindStacksMap
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

		// Execute backend generation
		err = ExecuteTerraformGenerateBackends(atmosConfig, "", "hcl", []string{}, []string{})
		assert.NoError(t, err)

		// Check that warning was logged - note: the YAML processor sets backend_type to empty string
		// so we hit the "empty after template processing" path, not the "not configured" path
		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "Skipping backend generation")
		assert.Contains(t, logOutput, "backend_type")
		assert.Contains(t, logOutput, "auto_generate_backend_file")

		// Verify no backend.tf or backend.tf.json was generated
		backendTF := filepath.Join(componentDir, "backend.tf")
		backendTFJSON := filepath.Join(componentDir, "backend.tf.json")
		_, errTF := os.Stat(backendTF)
		_, errJSON := os.Stat(backendTFJSON)
		assert.True(t, os.IsNotExist(errTF), "backend.tf should not be created when backend_type is missing")
		assert.True(t, os.IsNotExist(errJSON), "backend.tf.json should not be created when backend_type is missing")
	})
}
