package exec

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestStackManifestNameInStacksMap verifies that the 'name' field
// is correctly included in the processed stacks map.
func TestStackManifestNameInStacksMap(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

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

// describeStacksHelper is a helper function that calls ExecuteDescribeStacks with default parameters.
func describeStacksHelper(t *testing.T, atmosConfig *schema.AtmosConfiguration) map[string]any {
	t.Helper()
	result, err := ExecuteDescribeStacks(
		atmosConfig,
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
	return result
}

// getVpcWorkspaceFromStack extracts the workspace of the vpc component from a stack in the result map.
func getVpcWorkspaceFromStack(t *testing.T, result map[string]any, stackName string) string {
	t.Helper()
	stack, ok := result[stackName].(map[string]any)
	require.True(t, ok, "Stack '%s' should exist", stackName)

	components, ok := stack["components"].(map[string]any)
	require.True(t, ok, "Stack should have components")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "Stack should have terraform components")

	vpc, ok := terraform["vpc"].(map[string]any)
	require.True(t, ok, "Stack should have vpc component")

	workspace, ok := vpc["workspace"].(string)
	require.True(t, ok, "VPC component should have workspace")

	return workspace
}

// TestStackManifestName verifies that the 'name' field in stack manifests
// takes precedence over name_template and name_pattern from atmos.yaml.
func TestStackManifestName(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	result := describeStacksHelper(t, &atmosConfig)

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

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	result := describeStacksHelper(t, &atmosConfig)
	workspace := getVpcWorkspaceFromStack(t, result, "my-legacy-prod-stack")

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

// TestDescribeStacks_NameTemplate verifies that ExecuteDescribeStacks
// uses name_template when no explicit 'name' is set in the stack manifest.
func TestDescribeStacks_NameTemplate(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify that name_template is configured.
	require.NotEmpty(t, atmosConfig.Stacks.NameTemplate, "name_template should be configured in atmos.yaml")

	result := describeStacksHelper(t, &atmosConfig)

	// Verify that the stack with explicit 'name' field uses the custom name (highest precedence).
	_, hasExplicitName := result["my-explicit-stack"]
	assert.True(t, hasExplicitName, "Stack with 'name: my-explicit-stack' should use the explicit name as its key")

	// Verify that the stack without 'name' field uses name_template (prod-ue2).
	_, hasTemplatedName := result["prod-ue2"]
	assert.True(t, hasTemplatedName, "Stack without 'name' field should use name_template 'prod-ue2' as its key")

	// Verify that the original filenames are NOT used.
	_, hasOriginalExplicit := result["with-explicit-name"]
	assert.False(t, hasOriginalExplicit, "Stack with 'name' field should NOT use the original filename 'with-explicit-name'")

	_, hasOriginalWithout := result["without-explicit-name"]
	assert.False(t, hasOriginalWithout, "Stack without 'name' should NOT use the original filename 'without-explicit-name'")
}

// TestDescribeStacks_NameTemplateWorkspace verifies that the terraform workspace
// uses name_template when no explicit 'name' is set.
func TestDescribeStacks_NameTemplateWorkspace(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	result := describeStacksHelper(t, &atmosConfig)

	// The workspace should be based on the templated stack name.
	workspace := getVpcWorkspaceFromStack(t, result, "prod-ue2")
	assert.Equal(t, "prod-ue2", workspace, "Workspace should be based on the templated stack name")

	// The explicit name stack should use the explicit stack name, not name_template.
	explicitWorkspace := getVpcWorkspaceFromStack(t, result, "my-explicit-stack")
	assert.Equal(t, "my-explicit-stack", explicitWorkspace, "Workspace should use explicit stack name over name_template")
}

// TestDescribeStacks_NamePattern verifies that ExecuteDescribeStacks
// uses name_pattern when no explicit 'name' or 'name_template' is set.
func TestDescribeStacks_NamePattern(t *testing.T) {
	// Change to the test fixture directory with name_pattern configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-pattern"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify that name_pattern is configured but name_template is not.
	require.Empty(t, atmosConfig.Stacks.NameTemplate, "name_template should NOT be configured in atmos.yaml")
	require.NotEmpty(t, GetStackNamePattern(&atmosConfig), "name_pattern should be configured in atmos.yaml")

	result := describeStacksHelper(t, &atmosConfig)

	// Verify that the stack with explicit 'name' field uses the custom name (highest precedence).
	_, hasExplicitName := result["my-explicit-stack"]
	assert.True(t, hasExplicitName, "Stack with 'name: my-explicit-stack' should use the explicit name as its key")

	// Verify that the stack without 'name' field uses name_pattern (dev-uw2).
	_, hasPatternName := result["dev-uw2"]
	assert.True(t, hasPatternName, "Stack without 'name' field should use name_pattern 'dev-uw2' as its key")

	// Verify that the original filenames are NOT used.
	_, hasOriginalExplicit := result["with-explicit-name"]
	assert.False(t, hasOriginalExplicit, "Stack with 'name' field should NOT use the original filename 'with-explicit-name'")

	_, hasOriginalWithout := result["without-explicit-name"]
	assert.False(t, hasOriginalWithout, "Stack without 'name' should NOT use the original filename 'without-explicit-name'")
}

// TestDescribeStacks_NamePatternWorkspace verifies that the terraform workspace
// uses name_pattern when no explicit 'name' or 'name_template' is set.
func TestDescribeStacks_NamePatternWorkspace(t *testing.T) {
	// Change to the test fixture directory with name_pattern configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-pattern"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	// This also disables parent directory search and git root discovery.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	result := describeStacksHelper(t, &atmosConfig)

	// The workspace should be based on the pattern-derived stack name.
	workspace := getVpcWorkspaceFromStack(t, result, "dev-uw2")
	assert.Equal(t, "dev-uw2", workspace, "Workspace should be based on the pattern-derived stack name")
}

// TestProcessStacks_FindsComponentByManifestName verifies that ProcessStacks can find
// a component when the user specifies the manifest-level 'name' field as the stack argument.
//
// Scenario:
// - atmos.yaml has: name_template: "{{ .vars.environment }}-{{ .vars.stage }}"
// - Stack file: with-explicit-name.yaml has: name: "my-explicit-stack"
// - Stack vars: environment=prod, stage=ue1 (so template produces "prod-ue1")
// - User runs: atmos tf plan vpc -s my-explicit-stack (using manifest name, not template result)
//
// The manifest 'name' field should take precedence and allow the user to reference the stack
// by that name in all commands (terraform, helmfile, describe, etc.).
func TestProcessStacks_FindsComponentByManifestName(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config with the manifest name as the stack argument.
	// This simulates: atmos tf plan vpc -s my-explicit-stack
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "my-explicit-stack", // User specifies the manifest name override
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify the fixture is configured correctly.
	require.NotEmpty(t, atmosConfig.Stacks.NameTemplate, "name_template should be configured in atmos.yaml")

	// ProcessStacks should find the vpc component when user specifies the manifest name.
	result, err := ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	// The component should be found when using the manifest 'name' field.
	require.NoError(t, err, "ProcessStacks should find component when using manifest 'name' field; got error: %v", err)

	// Verify we found the correct component and stack.
	assert.Equal(t, "vpc", result.ComponentFromArg, "Component should be 'vpc'")
	// The Stack field should reflect what the user requested or the resolved stack name.
	assert.NotEmpty(t, result.StackFile, "StackFile should be set after finding the component")
}

// TestProcessStacks_FindsComponentByTemplateName verifies that ProcessStacks can find
// a component when the user specifies the template-derived name as the stack argument.
func TestProcessStacks_FindsComponentByTemplateName(t *testing.T) {
	// Change to the test fixture directory.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Use the template-derived name "prod-ue2" (from environment=prod, stage=ue2).
	// This tests the "without-explicit-name" stack which has no 'name' field,
	// so its logical name IS "prod-ue2" (environment=prod, stage=ue2).
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "prod-ue2", // Template-derived name for without-explicit-name stack
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// This should work - using the template-derived name is the current behavior.
	result, err := ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	require.NoError(t, err, "ProcessStacks should find component when using template name; got error: %v", err)
	assert.Equal(t, "vpc", result.ComponentFromArg, "Component should be 'vpc'")
	assert.NotEmpty(t, result.StackFile, "StackFile should be set after finding the component")
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

// =============================================================================
// Stack Name Identity Tests
// =============================================================================
// These tests verify that each stack has exactly ONE valid identifier.
// When an explicit 'name' field is set, only that name should work.
// When name_template/name_pattern is set (without explicit name), only the
// generated name should work. The filename is only valid when nothing else
// is configured.
//
// See docs/prd/stack-name-identity.md for the full specification.
// =============================================================================

// TestProcessStacks_RejectsGeneratedNameWhenExplicitNameSet verifies that when a stack
// has an explicit 'name' field, using the template-generated name should FAIL.
//
// Scenario:
// - Stack file: with-explicit-name.yaml
// - Manifest has: name: "my-explicit-stack"
// - name_template produces: "prod-ue1" (from environment=prod, stage=ue1)
// - User runs: atmos tf plan vpc -s prod-ue1 (using generated name)
//
// Expected: FAIL - only "my-explicit-stack" should be valid.
func TestProcessStacks_RejectsGeneratedNameWhenExplicitNameSet(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Try to use the template-generated name "prod-ue1" for a stack that has
	// explicit name "my-explicit-stack". This should FAIL.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "prod-ue1", // Generated name - should be rejected
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify fixture is configured correctly.
	require.NotEmpty(t, atmosConfig.Stacks.NameTemplate, "name_template should be configured")

	// ProcessStacks should NOT find the component when using generated name
	// for a stack with explicit name.
	_, err = ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	// This should fail because "prod-ue1" is not the canonical name.
	// The canonical name is "my-explicit-stack".
	assert.Error(t, err, "ProcessStacks should reject generated name when explicit name is set")
}

// TestProcessStacks_RejectsFilenameWhenExplicitNameSet verifies that when a stack
// has an explicit 'name' field, using the filename should FAIL.
//
// Scenario:
// - Stack file: with-explicit-name.yaml
// - Manifest has: name: "my-explicit-stack"
// - User runs: atmos tf plan vpc -s with-explicit-name (using filename)
//
// Expected: FAIL - only "my-explicit-stack" should be valid.
func TestProcessStacks_RejectsFilenameWhenExplicitNameSet(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Try to use the filename "with-explicit-name" for a stack that has
	// explicit name "my-explicit-stack". This should FAIL.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "with-explicit-name", // Filename - should be rejected
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// ProcessStacks should NOT find the component when using filename
	// for a stack with explicit name.
	_, err = ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	// This should fail because "with-explicit-name" is not the canonical name.
	// The canonical name is "my-explicit-stack".
	assert.Error(t, err, "ProcessStacks should reject filename when explicit name is set")
}

// TestProcessStacks_RejectsFilenameWhenTemplateSet verifies that when name_template
// is configured (and no explicit name), using the filename should FAIL.
//
// Scenario:
// - Stack file: without-explicit-name.yaml
// - No 'name' field in manifest
// - name_template produces: "prod-ue2" (from environment=prod, stage=ue2)
// - User runs: atmos tf plan vpc -s without-explicit-name (using filename)
//
// Expected: FAIL - only "prod-ue2" should be valid.
func TestProcessStacks_RejectsFilenameWhenTemplateSet(t *testing.T) {
	// Change to the test fixture directory with name_template configured.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name-template"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Try to use the filename "without-explicit-name" when name_template is set.
	// The canonical name should be "prod-ue2" (from template).
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "without-explicit-name", // Filename - should be rejected
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify fixture is configured correctly.
	require.NotEmpty(t, atmosConfig.Stacks.NameTemplate, "name_template should be configured")

	// ProcessStacks should NOT find the component when using filename
	// when name_template is configured.
	_, err = ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	// This should fail because "without-explicit-name" is not the canonical name.
	// The canonical name is "prod-ue2" (from name_template).
	assert.Error(t, err, "ProcessStacks should reject filename when name_template is set")
}

// TestProcessStacks_AcceptsFilenameWhenNoNamingConfigured verifies that when no
// name, name_template, or name_pattern is configured, the filename is the valid identifier.
//
// Scenario:
// - Stack file: no-name-prod.yaml
// - No 'name' field in manifest
// - No name_template in atmos.yaml
// - No name_pattern in atmos.yaml
// - User runs: atmos tf plan vpc -s no-name-prod (using filename)
//
// Expected: SUCCESS - filename is the only valid identifier.
func TestProcessStacks_AcceptsFilenameWhenNoNamingConfigured(t *testing.T) {
	// Change to the test fixture directory with NO name_template or name_pattern.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Use the filename "no-name-prod" for a stack with no naming configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "no-name-prod", // Filename - should work
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify fixture is configured correctly - no naming config.
	require.Empty(t, atmosConfig.Stacks.NameTemplate, "name_template should NOT be configured")
	require.Empty(t, GetStackNamePattern(&atmosConfig), "name_pattern should NOT be configured")

	// ProcessStacks should find the component when using filename
	// and no naming configuration exists.
	result, err := ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	require.NoError(t, err, "ProcessStacks should accept filename when no naming is configured")
	assert.Equal(t, "vpc", result.ComponentFromArg, "Component should be 'vpc'")
	assert.Equal(t, "no-name-prod", result.StackFile, "StackFile should be 'no-name-prod'")
}

// TestDescribeStacks_FilenameAsKeyWhenNoNamingConfigured verifies that ExecuteDescribeStacks
// returns the filename as the map key when no naming configuration exists.
//
// This test ensures that `atmos list stacks` and similar commands show the correct
// stack identifiers based on the naming precedence rules.
func TestDescribeStacks_FilenameAsKeyWhenNoNamingConfigured(t *testing.T) {
	// Change to the test fixture directory with NO name_template or name_pattern.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Initialize the CLI config.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// Verify fixture is configured correctly - no naming config.
	require.Empty(t, atmosConfig.Stacks.NameTemplate, "name_template should NOT be configured")

	result := describeStacksHelper(t, &atmosConfig)

	// Stack with explicit 'name' field should use that name as the key.
	_, hasExplicitName := result["my-legacy-prod-stack"]
	assert.True(t, hasExplicitName, "Stack with 'name: my-legacy-prod-stack' should use explicit name as key")

	// Stack with explicit 'name' should NOT appear under filename.
	_, hasFilename := result["legacy-prod"]
	assert.False(t, hasFilename, "Stack with explicit 'name' should NOT appear under filename 'legacy-prod'")

	// Stack without 'name' field should use filename as the key.
	_, hasNoNameStack := result["no-name-prod"]
	assert.True(t, hasNoNameStack, "Stack without 'name' field should use filename 'no-name-prod' as key")
}

// TestProcessStacks_InvalidStackErrorWithSuggestion verifies that when a user provides
// a filename for a stack that has an explicit name, the error message suggests the
// correct canonical name.
//
// Scenario:
// - Stack file: legacy-prod.yaml
// - Manifest has: name: "my-legacy-prod-stack"
// - User runs: atmos tf plan vpc -s legacy-prod (using filename)
//
// Expected: Error message should say "invalid stack" and suggest "my-legacy-prod-stack".
func TestProcessStacks_InvalidStackErrorWithSuggestion(t *testing.T) {
	// Change to the test fixture directory with NO name_template or name_pattern.
	testDir := "../../tests/fixtures/scenarios/stack-manifest-name"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Try to use the filename "legacy-prod" for a stack that has
	// explicit name "my-legacy-prod-stack". This should FAIL with a helpful suggestion.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "legacy-prod", // Filename - should be rejected with suggestion
		ComponentType:    cfg.TerraformComponentType,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	require.NoError(t, err)

	// ProcessStacks should fail with ErrInvalidStack (not ErrInvalidComponent).
	_, err = ProcessStacks(&atmosConfig, configAndStacksInfo, true, false, false, nil, nil)

	// Verify error is ErrInvalidStack, not ErrInvalidComponent.
	assert.ErrorIs(t, err, errUtils.ErrInvalidStack, "Error should be ErrInvalidStack when filename is used for stack with explicit name")

	// Verify the hints suggest the correct canonical name and list stacks command.
	hints := errors.GetAllHints(err)
	require.Len(t, hints, 2, "Error should have exactly 2 hints")

	// Check for the "Did you mean" hint.
	assert.Equal(t, "Did you mean `my-legacy-prod-stack`?", hints[0], "First hint should suggest the canonical name")

	// Check for the "list stacks" hint.
	assert.Equal(t, "Run `atmos list stacks` to see all available stacks.", hints[1], "Second hint should suggest listing stacks")
}
