package tests

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/terraform/shared"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestComponentMenuFilteringAbstractComponents tests that abstract components
// (metadata.type: abstract) are filtered out from component listings.
// This tests the fix for PR #1977.
func TestComponentMenuFilteringAbstractComponents(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack - get all stacks
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, stacksMap)

	// Verify vpc-base is marked as abstract in the dev stack.
	devStack, ok := stacksMap["dev"].(map[string]any)
	require.True(t, ok, "dev stack should exist")

	components, ok := devStack["components"].(map[string]any)
	require.True(t, ok, "dev stack should have components")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "dev stack should have terraform components")

	// vpc-base should exist and be marked as abstract.
	vpcBase, ok := terraform["vpc-base"].(map[string]any)
	require.True(t, ok, "vpc-base should exist in dev stack")

	metadata, ok := vpcBase["metadata"].(map[string]any)
	require.True(t, ok, "vpc-base should have metadata")

	metadataType, ok := metadata["type"].(string)
	require.True(t, ok, "vpc-base metadata should have type")
	assert.Equal(t, "abstract", metadataType, "vpc-base should be abstract")

	// Verify that isComponentDeployable correctly identifies abstract components.
	// Test the helper function directly.
	assert.False(t, shared.IsComponentDeployable(vpcBase),
		"vpc-base (abstract) should NOT be deployable")

	// Regular vpc component should be deployable.
	vpc, ok := terraform["vpc"].(map[string]any)
	require.True(t, ok, "vpc should exist in dev stack")
	assert.True(t, shared.IsComponentDeployable(vpc),
		"vpc should be deployable")
}

// TestComponentMenuFilteringDisabledComponents tests that disabled components
// (metadata.enabled: false) are filtered out from component listings.
// This tests the fix for PR #1977.
func TestComponentMenuFilteringDisabledComponents(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack - get all stacks
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, stacksMap)

	// Verify eks is disabled in the dev stack.
	devStack, ok := stacksMap["dev"].(map[string]any)
	require.True(t, ok, "dev stack should exist")

	components, ok := devStack["components"].(map[string]any)
	require.True(t, ok, "dev stack should have components")

	terraform, ok := components["terraform"].(map[string]any)
	require.True(t, ok, "dev stack should have terraform components")

	// eks should exist in dev stack and be disabled.
	eks, ok := terraform["eks"].(map[string]any)
	require.True(t, ok, "eks should exist in dev stack")

	metadata, ok := eks["metadata"].(map[string]any)
	require.True(t, ok, "eks should have metadata in dev stack")

	enabled, ok := metadata["enabled"].(bool)
	require.True(t, ok, "eks metadata should have enabled field")
	assert.False(t, enabled, "eks should be disabled in dev stack")

	// Verify that isComponentDeployable correctly identifies disabled components.
	assert.False(t, shared.IsComponentDeployable(eks),
		"eks (disabled in dev) should NOT be deployable")

	// Verify eks is NOT disabled in prod stack.
	prodStack, ok := stacksMap["prod"].(map[string]any)
	require.True(t, ok, "prod stack should exist")

	prodComponents, ok := prodStack["components"].(map[string]any)
	require.True(t, ok, "prod stack should have components")

	prodTerraform, ok := prodComponents["terraform"].(map[string]any)
	require.True(t, ok, "prod stack should have terraform components")

	prodEks, ok := prodTerraform["eks"].(map[string]any)
	require.True(t, ok, "eks should exist in prod stack")

	// eks in prod should be deployable (not disabled).
	assert.True(t, shared.IsComponentDeployable(prodEks),
		"eks in prod stack should be deployable")
}

// TestComponentMenuFilteringStackScoped tests that when a stack is specified,
// only components deployed in that specific stack appear.
// This tests the fix for PR #1977.
func TestComponentMenuFilteringStackScoped(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	tests := []struct {
		name               string
		stack              string
		expectedComponents []string
		description        string
	}{
		{
			name:               "dev stack has vpc only (eks disabled, no rds)",
			stack:              "dev",
			expectedComponents: []string{"vpc"},
			description:        "dev stack should only show vpc (eks is disabled, rds not present)",
		},
		{
			name:               "staging stack has vpc and rds (no eks)",
			stack:              "staging",
			expectedComponents: []string{"rds", "vpc"},
			description:        "staging stack should show rds and vpc (eks not present in this stack)",
		},
		{
			name:               "prod stack has all components",
			stack:              "prod",
			expectedComponents: []string{"eks", "rds", "vpc"},
			description:        "prod stack should show all deployable components",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get stack-specific configuration.
			stacksMap, err := exec.ExecuteDescribeStacks(
				&atmosConfig,
				tt.stack, // filterByStack
				nil,      // components
				nil,      // componentTypes
				nil,      // sections
				true,     // ignoreMissingFiles
				true,     // processTemplates
				true,     // processYamlFunctions
				false,    // includeEmptyStacks
				nil,      // skip
				nil,      // authManager
			)
			require.NoError(t, err, tt.description)
			require.NotNil(t, stacksMap, tt.description)

			// Get the specific stack.
			stackData, ok := stacksMap[tt.stack].(map[string]any)
			require.True(t, ok, "stack %s should exist", tt.stack)

			components, ok := stackData["components"].(map[string]any)
			require.True(t, ok, "stack %s should have components", tt.stack)

			terraform, ok := components["terraform"].(map[string]any)
			require.True(t, ok, "stack %s should have terraform components", tt.stack)

			// Filter to only deployable components.
			deployable := shared.FilterDeployableComponents(terraform)
			sort.Strings(deployable)

			assert.Equal(t, tt.expectedComponents, deployable, tt.description)
		})
	}
}

// TestComponentMenuFilteringAllStacks tests that when no stack is specified,
// all deployable components across all stacks are returned.
// This tests the fix for PR #1977.
func TestComponentMenuFilteringAllStacks(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack - get all stacks
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, stacksMap)

	// Collect unique deployable component names from all stacks.
	componentSet := make(map[string]struct{})
	for _, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					// Filter to only deployable components.
					deployable := shared.FilterDeployableComponents(terraform)
					for _, name := range deployable {
						componentSet[name] = struct{}{}
					}
				}
			}
		}
	}

	// Convert to sorted slice.
	var allComponents []string
	for name := range componentSet {
		allComponents = append(allComponents, name)
	}
	sort.Strings(allComponents)

	// Expected: eks, rds, vpc (but NOT vpc-base which is abstract).
	expected := []string{"eks", "rds", "vpc"}
	assert.Equal(t, expected, allComponents,
		"all stacks should have eks, rds, vpc as deployable components (vpc-base filtered out)")
}

// TestComponentMenuFilteringVerifyStackStructure verifies the test fixture structure
// matches what was tested manually via shell commands.
func TestComponentMenuFilteringVerifyStackStructure(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack - get all stacks
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)
	require.NotNil(t, stacksMap)

	// Verify the fixture has exactly 3 stacks: dev, staging, prod.
	var stackNames []string
	for stackName := range stacksMap {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)
	assert.Equal(t, []string{"dev", "prod", "staging"}, stackNames,
		"fixture should have exactly dev, prod, staging stacks")

	// Verify component presence matches documentation.
	componentPresence := map[string]map[string]struct{}{
		"dev":     {},
		"staging": {},
		"prod":    {},
	}

	for stackName, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					for componentName := range terraform {
						componentPresence[stackName][componentName] = struct{}{}
					}
				}
			}
		}
	}

	// Verify dev stack has: vpc-base, vpc, eks (but eks is disabled).
	_, hasVpcBase := componentPresence["dev"]["vpc-base"]
	_, hasVpc := componentPresence["dev"]["vpc"]
	_, hasEks := componentPresence["dev"]["eks"]
	_, hasRds := componentPresence["dev"]["rds"]
	assert.True(t, hasVpcBase, "dev should have vpc-base (abstract)")
	assert.True(t, hasVpc, "dev should have vpc")
	assert.True(t, hasEks, "dev should have eks (disabled)")
	assert.False(t, hasRds, "dev should NOT have rds")

	// Verify staging stack has: vpc-base, vpc, rds (no eks).
	_, hasVpcBase = componentPresence["staging"]["vpc-base"]
	_, hasVpc = componentPresence["staging"]["vpc"]
	_, hasRds = componentPresence["staging"]["rds"]
	_, hasEks = componentPresence["staging"]["eks"]
	assert.True(t, hasVpcBase, "staging should have vpc-base (abstract)")
	assert.True(t, hasVpc, "staging should have vpc")
	assert.True(t, hasRds, "staging should have rds")
	assert.False(t, hasEks, "staging should NOT have eks")

	// Verify prod stack has: vpc-base, vpc, eks, rds.
	_, hasVpcBase = componentPresence["prod"]["vpc-base"]
	_, hasVpc = componentPresence["prod"]["vpc"]
	_, hasEks = componentPresence["prod"]["eks"]
	_, hasRds = componentPresence["prod"]["rds"]
	assert.True(t, hasVpcBase, "prod should have vpc-base (abstract)")
	assert.True(t, hasVpc, "prod should have vpc")
	assert.True(t, hasEks, "prod should have eks")
	assert.True(t, hasRds, "prod should have rds")
}

// TestFilterDeployableComponentsFixture tests the FilterDeployableComponents helper
// using the actual fixture data.
func TestFilterDeployableComponentsFixture(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack - get all stacks
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)

	tests := []struct {
		stack              string
		expectedDeployable []string
	}{
		{
			stack:              "dev",
			expectedDeployable: []string{"vpc"}, // eks disabled, vpc-base abstract, no rds
		},
		{
			stack:              "staging",
			expectedDeployable: []string{"rds", "vpc"}, // vpc-base abstract, no eks
		},
		{
			stack:              "prod",
			expectedDeployable: []string{"eks", "rds", "vpc"}, // vpc-base abstract
		},
	}

	for _, tt := range tests {
		t.Run(tt.stack, func(t *testing.T) {
			stackData := stacksMap[tt.stack].(map[string]any)
			components := stackData["components"].(map[string]any)
			terraform := components["terraform"].(map[string]any)

			deployable := shared.FilterDeployableComponents(terraform)
			assert.Equal(t, tt.expectedDeployable, deployable)
		})
	}
}

// TestIsComponentDeployableFixture tests the IsComponentDeployable helper
// using the actual fixture data.
func TestIsComponentDeployableFixture(t *testing.T) {
	t.Chdir("./fixtures/scenarios/component-menu-filtering")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get dev stack which has vpc-base (abstract) and eks (disabled).
	stacksMap, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"dev", // filterByStack
		nil,   // components
		nil,   // componentTypes
		nil,   // sections
		true,  // ignoreMissingFiles
		true,  // processTemplates
		true,  // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
	)
	require.NoError(t, err)

	devStack := stacksMap["dev"].(map[string]any)
	components := devStack["components"].(map[string]any)
	terraform := components["terraform"].(map[string]any)

	tests := []struct {
		component          string
		expectedDeployable bool
		reason             string
	}{
		{
			component:          "vpc-base",
			expectedDeployable: false,
			reason:             "abstract component",
		},
		{
			component:          "vpc",
			expectedDeployable: true,
			reason:             "regular deployable component",
		},
		{
			component:          "eks",
			expectedDeployable: false,
			reason:             "disabled component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			componentConfig := terraform[tt.component]
			result := shared.IsComponentDeployable(componentConfig)
			assert.Equal(t, tt.expectedDeployable, result,
				"%s should %sbe deployable (%s)",
				tt.component,
				map[bool]string{true: "", false: "NOT "}[tt.expectedDeployable],
				tt.reason)
		})
	}
}
