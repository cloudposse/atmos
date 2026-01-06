package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLocalsResolutionDev tests that file-scoped locals are properly resolved in dev environment.
// This is an integration test for GitHub issue #1933.
func TestLocalsResolutionDev(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration with locals resolved.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "mock/instance-1",
		Stack:                "dev-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify locals were resolved correctly in vars.
	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Check that {{ .locals.name_prefix }}-mock-instance-1 resolved to "acme-dev-mock-instance-1".
	assert.Equal(t, "acme-dev-mock-instance-1", vars["app_name"], "app_name should be resolved from locals")

	// Check that {{ .locals.environment }} resolved to "dev".
	assert.Equal(t, "dev", vars["bar"], "bar should be resolved from locals.environment")

	// Check that {{ .locals.backend_bucket }} resolved to "acme-dev-tfstate".
	assert.Equal(t, "acme-dev-tfstate", vars["bucket"], "bucket should be resolved from locals.backend_bucket")

	// Verify backend was also resolved.
	backend, ok := result["backend"].(map[string]any)
	require.True(t, ok, "backend should be a map")
	assert.Equal(t, "acme-dev-tfstate", backend["bucket"], "backend bucket should be resolved from locals")
}

// TestLocalsResolutionProd tests locals resolution in the prod environment.
func TestLocalsResolutionProd(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	_, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Get component configuration with locals resolved.
	result, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		Component:            "mock/primary",
		Stack:                "prod-us-east-1",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify locals were resolved correctly in vars.
	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map")

	// Check that {{ .locals.name_prefix }}-mock-primary resolved to "acme-prod-mock-primary".
	assert.Equal(t, "acme-prod-mock-primary", vars["app_name"], "app_name should be resolved from locals")

	// Check that {{ .locals.environment }} resolved to "prod".
	assert.Equal(t, "prod", vars["bar"], "bar should be resolved from locals.environment")

	// Check that {{ .locals.backend_bucket }} resolved to "acme-prod-tfstate".
	assert.Equal(t, "acme-prod-tfstate", vars["bucket"], "bucket should be resolved from locals.backend_bucket")
}

// TestLocalsDescribeStacks tests that describe stacks works with locals.
func TestLocalsDescribeStacks(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks configuration.
	result, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack
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
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack")

	// Find a stack that contains the mock/instance-1 component.
	var foundStack map[string]any
	for _, stackData := range result {
		stack, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		components, ok := stack["components"].(map[string]any)
		if !ok {
			continue
		}
		terraform, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}
		if _, exists := terraform["mock/instance-1"]; exists {
			foundStack = stack
			break
		}
	}
	require.NotNil(t, foundStack, "should find a stack with mock/instance-1 component")

	components := foundStack["components"].(map[string]any)
	terraform := components["terraform"].(map[string]any)
	mockInstance1 := terraform["mock/instance-1"].(map[string]any)
	vars := mockInstance1["vars"].(map[string]any)

	// Verify locals were resolved.
	assert.Equal(t, "acme-dev-mock-instance-1", vars["app_name"], "app_name should be resolved")
	assert.Equal(t, "dev", vars["bar"], "bar should be resolved")
}

// TestLocalsCircularDependency verifies that circular locals don't crash the system.
// When locals have a cycle, the resolver should log an error and continue without locals.
func TestLocalsCircularDependency(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-circular")

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Get all stacks - should succeed even with circular locals.
	// The circular locals are logged as a debug warning but processing continues.
	result, err := exec.ExecuteDescribeStacks(
		&atmosConfig,
		"",    // filterByStack
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

	// Should not error - circular locals are handled gracefully.
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result, "should have at least one stack")

	// Find a stack that contains the mock component.
	var foundStack map[string]any
	for _, stackData := range result {
		stack, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		components, ok := stack["components"].(map[string]any)
		if !ok {
			continue
		}
		terraform, ok := components["terraform"].(map[string]any)
		if !ok {
			continue
		}
		if _, exists := terraform["mock"]; exists {
			foundStack = stack
			break
		}
	}
	require.NotNil(t, foundStack, "should find a stack with mock component")

	components := foundStack["components"].(map[string]any)
	terraform := components["terraform"].(map[string]any)

	// The mock component should exist.
	_, ok := terraform["mock"].(map[string]any)
	require.True(t, ok, "mock component should exist")
}
