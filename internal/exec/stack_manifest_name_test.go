package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestStackManifestNameInStacksMap verifies that the 'name' field
// is correctly included in the processed stacks map.
func TestStackManifestNameInStacksMap(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Call FindStacksMap to get the processed stacks.
	stacksMap, _, err := FindStacksMap(&atmosConfig, false)
	require.NoError(t, err)
	require.NotNil(t, stacksMap)

	// Check that legacy-prod stack exists and has the 'name' field.
	legacyProdStack, ok := stacksMap["legacy-prod"]
	require.True(t, ok, "Stack 'legacy-prod' should exist in stacks map")

	legacyProdStackMap, ok := legacyProdStack.(map[string]any)
	require.True(t, ok, "Stack should be a map")

	// Check for the 'name' field.
	nameValue, hasName := legacyProdStackMap["name"]
	t.Logf("Stack 'legacy-prod' contents (keys): %v", getMapKeys(legacyProdStackMap))
	t.Logf("Stack 'legacy-prod' has 'name' field: %v, value: %v", hasName, nameValue)

	assert.True(t, hasName, "Stack 'legacy-prod' should have 'name' field")
	if hasName {
		assert.Equal(t, "my-legacy-prod-stack", nameValue, "Stack 'name' field should be 'my-legacy-prod-stack'")
	}
}

// getMapKeys returns the keys of a map as a slice.
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestStackManifestName verifies that the 'name' field in stack manifests
// takes precedence over name_template and name_pattern from atmos.yaml.
func TestStackManifestName(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Call ExecuteDescribeStacks.
	result, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",         // filterByStack
		[]string{}, // components
		[]string{}, // componentTypes
		[]string{}, // sections
		false,      // ignoreMissingFiles
		false,      // processTemplates
		false,      // processYamlFunctions
		false,      // includeEmptyStacks
		[]string{}, // skip
		nil,        // authManager
	)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify that the stack with 'name' field uses the custom name.
	_, hasCustomName := result["my-legacy-prod-stack"]
	assert.True(t, hasCustomName, "Stack with 'name: my-legacy-prod-stack' should use the custom name as its key")

	// Verify that the stack without 'name' field uses the filename.
	_, hasDefaultName := result["no-name-prod"]
	assert.True(t, hasDefaultName, "Stack without 'name' field should use the filename 'no-name-prod' as its key")

	// Verify that the original filename is NOT used for the stack with custom name.
	_, hasOriginalName := result["legacy-prod"]
	assert.False(t, hasOriginalName, "Stack with 'name' field should NOT use the original filename 'legacy-prod'")
}

// TestStackManifestNameWorkspace verifies that the terraform workspace
// also respects the 'name' field from stack manifests.
func TestStackManifestNameWorkspace(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Call ExecuteDescribeStacks.
	result, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",         // filterByStack
		[]string{}, // components
		[]string{}, // componentTypes
		[]string{}, // sections
		false,      // ignoreMissingFiles
		false,      // processTemplates
		false,      // processYamlFunctions
		false,      // includeEmptyStacks
		[]string{}, // skip
		nil,        // authManager
	)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Get the stack with custom name and check its workspace.
	customStack, ok := result["my-legacy-prod-stack"].(map[string]any)
	require.True(t, ok, "Stack 'my-legacy-prod-stack' should exist")

	components, ok := customStack["components"].(map[string]any)
	require.True(t, ok, "Stack should have components")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "Stack should have terraform components")

	vpc, ok := terraform["vpc"].(map[string]any)
	require.True(t, ok, "Stack should have vpc component")

	workspace, ok := vpc["workspace"].(string)
	require.True(t, ok, "VPC component should have workspace")

	// The workspace should be based on the custom stack name.
	assert.Equal(t, "my-legacy-prod-stack", workspace, "Workspace should be based on the custom stack name")
}

// TestBuildTerraformWorkspace_StackManifestName tests that BuildTerraformWorkspace
// uses StackManifestName when set (highest precedence).
func TestBuildTerraformWorkspace_StackManifestName(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "{{ .vars.environment }}-{{ .vars.stage }}", // Should be ignored.
			NamePattern:  "{environment}-{stage}",                     // Should be ignored.
		},
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		StackManifestName:        "my-explicit-stack-name",
		Stack:                    "some-stack-file",
		ComponentMetadataSection: map[string]any{},
		Context: schema.Context{
			Environment: "prod",
			Stage:       "us-east-1",
		},
	}

	workspace, err := BuildTerraformWorkspace(&atmosConfig, configAndStacksInfo)
	require.NoError(t, err)
	assert.Equal(t, "my-explicit-stack-name", workspace, "Workspace should use StackManifestName")
}

// TestBuildTerraformWorkspace_NameTemplate tests that BuildTerraformWorkspace
// uses name_template when StackManifestName is not set.
func TestBuildTerraformWorkspace_NameTemplate(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "{{ .vars.environment }}-{{ .vars.stage }}",
			NamePattern:  "{environment}-{stage}", // Should be ignored.
		},
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		StackManifestName:        "", // Not set.
		Stack:                    "some-stack-file",
		ComponentMetadataSection: map[string]any{},
		ComponentSection: map[string]any{
			"vars": map[string]any{
				"environment": "prod",
				"stage":       "ue1",
			},
		},
		Context: schema.Context{
			Environment: "prod",
			Stage:       "ue1",
		},
	}

	workspace, err := BuildTerraformWorkspace(&atmosConfig, configAndStacksInfo)
	require.NoError(t, err)
	assert.Equal(t, "prod-ue1", workspace, "Workspace should use name_template")
}

// TestBuildTerraformWorkspace_NamePattern tests that BuildTerraformWorkspace
// uses name_pattern when neither StackManifestName nor name_template is set.
func TestBuildTerraformWorkspace_NamePattern(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "", // Not set.
			NamePattern:  "{environment}-{stage}",
		},
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		StackManifestName:        "", // Not set.
		Stack:                    "some-stack-file",
		ComponentMetadataSection: map[string]any{},
		Context: schema.Context{
			Environment: "prod",
			Stage:       "ue1",
		},
	}

	workspace, err := BuildTerraformWorkspace(&atmosConfig, configAndStacksInfo)
	require.NoError(t, err)
	assert.Equal(t, "prod-ue1", workspace, "Workspace should use name_pattern")
}

// TestBuildTerraformWorkspace_DefaultFilename tests that BuildTerraformWorkspace
// falls back to the stack filename when no naming config is set.
func TestBuildTerraformWorkspace_DefaultFilename(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "", // Not set.
			NamePattern:  "", // Not set.
		},
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		StackManifestName:        "", // Not set.
		Stack:                    "prod/us-east-1",
		ComponentMetadataSection: map[string]any{},
		Context:                  schema.Context{},
	}

	workspace, err := BuildTerraformWorkspace(&atmosConfig, configAndStacksInfo)
	require.NoError(t, err)
	assert.Equal(t, "prod-us-east-1", workspace, "Workspace should use stack filename with / replaced by -")
}

// TestBuildTerraformWorkspace_Precedence verifies the full precedence order:
// name (manifest) > name_template > name_pattern > filename.
func TestBuildTerraformWorkspace_Precedence(t *testing.T) {
	tests := []struct {
		name              string
		stackManifestName string
		nameTemplate      string
		namePattern       string
		stackFilename     string
		expectedWorkspace string
	}{
		{
			name:              "manifest name takes precedence over all",
			stackManifestName: "explicit-name",
			nameTemplate:      "{{ .vars.env }}",
			namePattern:       "{environment}",
			stackFilename:     "fallback-file",
			expectedWorkspace: "explicit-name",
		},
		{
			name:              "name_template takes precedence over pattern and filename",
			stackManifestName: "",
			nameTemplate:      "template-result",
			namePattern:       "{environment}",
			stackFilename:     "fallback-file",
			expectedWorkspace: "template-result",
		},
		{
			name:              "name_pattern takes precedence over filename",
			stackManifestName: "",
			nameTemplate:      "",
			namePattern:       "{environment}-{stage}",
			stackFilename:     "fallback-file",
			expectedWorkspace: "prod-dev",
		},
		{
			name:              "filename used when nothing else configured",
			stackManifestName: "",
			nameTemplate:      "",
			namePattern:       "",
			stackFilename:     "my-stack-file",
			expectedWorkspace: "my-stack-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
				Stacks: schema.Stacks{
					NameTemplate: tt.nameTemplate,
					NamePattern:  tt.namePattern,
				},
			}

			configAndStacksInfo := schema.ConfigAndStacksInfo{
				StackManifestName:        tt.stackManifestName,
				Stack:                    tt.stackFilename,
				ComponentMetadataSection: map[string]any{},
				ComponentSection: map[string]any{
					"vars": map[string]any{
						"env": "template-result",
					},
				},
				Context: schema.Context{
					Environment: "prod",
					Stage:       "dev",
				},
			}

			workspace, err := BuildTerraformWorkspace(&atmosConfig, configAndStacksInfo)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedWorkspace, workspace)
		})
	}
}
