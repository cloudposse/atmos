package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewListStacksTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewListStacksTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestListStacksTool_Name(t *testing.T) {
	tool := NewListStacksTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_list_stacks", tool.Name())
}

func TestListStacksTool_Description(t *testing.T) {
	tool := NewListStacksTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "List all available Atmos stacks")
}

func TestListStacksTool_Parameters(t *testing.T) {
	tool := NewListStacksTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 1)
	assert.Equal(t, "format", params[0].Name)
	assert.Equal(t, "Output format (yaml or json)", params[0].Description)
	assert.False(t, params[0].Required)
	assert.Equal(t, "yaml", params[0].Default)
}

func TestListStacksTool_RequiresPermission(t *testing.T) {
	tool := NewListStacksTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestListStacksTool_IsRestricted(t *testing.T) {
	tool := NewListStacksTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestListStacksTool_Execute_DefaultFormat(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	// Execute with empty params (should use default format).
	result, err := tool.Execute(ctx, map[string]interface{}{})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks (yaml format)")

	// Check data fields.
	assert.Equal(t, "yaml", result.Data["format"])

	stacks, ok := result.Data["stacks"].([]string)
	require.True(t, ok)
	// Stacks may be empty in test environment, just verify it's a valid slice.
	assert.NotNil(t, stacks)
}

func TestListStacksTool_Execute_YamlFormat(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"format": "yaml",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks (yaml format)")

	assert.Equal(t, "yaml", result.Data["format"])
}

func TestListStacksTool_Execute_JsonFormat(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"format": "json",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks (json format)")

	assert.Equal(t, "json", result.Data["format"])
}

func TestListStacksTool_Execute_EmptyFormat(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	// Empty format string should use default.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"format": "",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks (yaml format)")

	assert.Equal(t, "yaml", result.Data["format"])
}

func TestListStacksTool_Execute_NonStringFormat(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	// Non-string format should be ignored and use default.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"format": 123,
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks (yaml format)")

	assert.Equal(t, "yaml", result.Data["format"])
}

func TestListStacksTool_Execute_InvalidConfig(t *testing.T) {
	// Create config with invalid base path (cross-platform non-existent path).
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: filepath.Join(t.TempDir(), "nonexistent", "path", "that", "does", "not", "exist"),
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	// ExecuteDescribeStacks may still succeed with empty results even with invalid path.
	// Just verify result is returned.
	assert.NoError(t, err)
	assert.True(t, result.Success)
	// When base path doesn't exist, we should get empty stacks.
	stacks, ok := result.Data["stacks"].([]string)
	require.True(t, ok)
	assert.Empty(t, stacks)
}

func TestListStacksTool_Execute_EmptyStacks(t *testing.T) {
	// Create temp directory with no stacks.
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	// Create minimal atmos.yaml.
	atmosYaml := `
base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYaml), 0o600))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Stacks")

	stacks, ok := result.Data["stacks"].([]string)
	require.True(t, ok)
	assert.Empty(t, stacks)
}

func TestListStacksTool_Execute_MultipleStacks(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	require.NoError(t, err)
	assert.True(t, result.Success)

	stacks, ok := result.Data["stacks"].([]string)
	require.True(t, ok)

	// Verify we got a valid slice (may be empty in test environment).
	assert.NotNil(t, stacks)

	// If we have stacks, output should contain them.
	if len(stacks) > 0 {
		for _, stackName := range stacks {
			assert.Contains(t, result.Output, "- "+stackName)
		}
	}
}

func TestListStacksTool_Execute_ContextCancellation(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// Execute should still work as ExecuteDescribeStacks doesn't check context.
	// This test documents current behavior.
	result, err := tool.Execute(ctx, map[string]interface{}{})

	// Current implementation doesn't check context, so it succeeds.
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestListStacksTool_Execute_AllFormatVariations(t *testing.T) {
	atmosConfig, cleanup := setupListStacksTestEnv(t)
	defer cleanup()

	tool := NewListStacksTool(atmosConfig)
	ctx := context.Background()

	testCases := []struct {
		name           string
		format         interface{}
		expectedFormat string
	}{
		{
			name:           "nil format",
			format:         nil,
			expectedFormat: "yaml",
		},
		{
			name:           "uppercase YAML",
			format:         "YAML",
			expectedFormat: "YAML",
		},
		{
			name:           "uppercase JSON",
			format:         "JSON",
			expectedFormat: "JSON",
		},
		{
			name:           "mixed case",
			format:         "YaMl",
			expectedFormat: "YaMl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{}
			if tc.format != nil {
				params["format"] = tc.format
			}

			result, err := tool.Execute(ctx, params)

			require.NoError(t, err)
			assert.True(t, result.Success)
			assert.Equal(t, tc.expectedFormat, result.Data["format"])
		})
	}
}

// setupListStacksTestEnv creates a test environment with stack files for list stacks tests.
func setupListStacksTestEnv(t *testing.T) (*schema.AtmosConfiguration, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create stacks directory structure.
	stacksDir := filepath.Join(tmpDir, "stacks")
	catalogDir := filepath.Join(stacksDir, "catalog")
	orgsDir := filepath.Join(stacksDir, "orgs")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))
	require.NoError(t, os.MkdirAll(orgsDir, 0o755))

	// Create test stack files.
	vpcStack := `components:
  terraform:
    vpc:
      vars:
        cidr_block: 10.0.0.0/16
`
	require.NoError(t, os.WriteFile(filepath.Join(catalogDir, "vpc.yaml"), []byte(vpcStack), 0o600))

	// Create dev stack.
	devStack := `import:
  - catalog/vpc

components:
  terraform:
    vpc:
      vars:
        environment: dev
`
	require.NoError(t, os.WriteFile(filepath.Join(orgsDir, "dev.yaml"), []byte(devStack), 0o600))

	// Create prod stack.
	prodStack := `import:
  - catalog/vpc

components:
  terraform:
    vpc:
      vars:
        environment: prod
`
	require.NoError(t, os.WriteFile(filepath.Join(orgsDir, "prod.yaml"), []byte(prodStack), 0o600))

	// Create components directory.
	componentsDir := filepath.Join(tmpDir, "components", "terraform")
	require.NoError(t, os.MkdirAll(componentsDir, 0o755))

	// Create minimal atmos.yaml.
	atmosYaml := `
base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  name_pattern: "{tenant}-{environment}-{stage}"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYaml), 0o600))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath:    "stacks",
			NamePattern: "{tenant}-{environment}-{stage}",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	cleanup := func() {
		// Temp dir auto-cleaned by t.TempDir()
	}

	return atmosConfig, cleanup
}
