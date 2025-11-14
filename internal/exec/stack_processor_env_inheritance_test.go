package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// findComponentInStacks searches for a specific component in the stacks map.
//
//nolint:nestif // Nested if statements are necessary for safe type assertion chains.
func findComponentInStacks(stacks map[string]any, componentKey string) (map[string]any, string, bool) {
	for stackName, stackConfig := range stacks {
		if stackMap, ok := stackConfig.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					if comp, ok := terraform[componentKey].(map[string]any); ok {
						return comp, stackName, true
					}
				}
			}
		}
	}
	return nil, "", false
}

// TestEnvInheritance_GlobalToComponent verifies that env variables defined in the global
// section of stack manifests are inherited by components.
func TestEnvInheritance_GlobalToComponent(t *testing.T) {
	// Set up test fixture path.
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "env-inheritance")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	// Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err, "Failed to initialize CLI config")

	// Process stacks to get component configurations.
	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.NoError(t, err, "Failed to process stacks")
	require.NotNil(t, stacks, "Stacks should not be nil")

	// Find the test-component-dev component.
	componentConfig, stackName, found := findComponentInStacks(stacks, "test-component-dev")
	require.True(t, found, "Component test-component-dev not found in any stack")
	require.NotNil(t, componentConfig, "Component config should not be nil")
	t.Logf("Found component test-component-dev in stack %s", stackName)

	// Extract env section from component config.
	envSection, ok := componentConfig["env"].(map[string]any)
	require.True(t, ok, "Component should have an env section")
	require.NotNil(t, envSection, "Env section should not be nil")

	// Test cases for env variable inheritance and override behavior.
	tests := []struct {
		name          string
		envVarName    string
		expectedValue string
		description   string
	}{
		{
			name:          "global env inherited",
			envVarName:    "GLOBAL_ENV_VAR",
			expectedValue: "global-value",
			description:   "Global env variables should be inherited by components",
		},
		{
			name:          "terraform global env inherited",
			envVarName:    "TF_GLOBAL_VAR",
			expectedValue: "terraform-global",
			description:   "Terraform-specific global env variables should be inherited",
		},
		{
			name:          "component env variable",
			envVarName:    "COMPONENT_ENV_VAR",
			expectedValue: "overridden-by-stack",
			description:   "Stack-level env should override component catalog env",
		},
		{
			name:          "stack env variable",
			envVarName:    "STACK_ENV_VAR",
			expectedValue: "stack-value",
			description:   "Stack-specific env variables should be present",
		},
		{
			name:          "global override by component",
			envVarName:    "GLOBAL_OVERRIDE_TEST",
			expectedValue: "overridden-by-stack",
			description:   "Stack env should override global env (stack has highest priority)",
		},
		{
			name:          "terraform global override by stack",
			envVarName:    "TF_OVERRIDE_TEST",
			expectedValue: "from-stack",
			description:   "Stack env should override terraform global env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, exists := envSection[tt.envVarName]
			assert.True(t, exists, "Env variable %s should exist: %s", tt.envVarName, tt.description)
			if exists {
				assert.Equal(t, tt.expectedValue, value, "Env variable %s value mismatch: %s", tt.envVarName, tt.description)
			}
		})
	}
}

// TestEnvInheritance_MergePriority verifies the merge priority of env variables:
// GlobalEnv < BaseComponentEnv < ComponentEnv < ComponentOverridesEnv.
func TestEnvInheritance_MergePriority(t *testing.T) {
	// Set up test fixture path.
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "env-inheritance")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	// Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err, "Failed to initialize CLI config")

	// Process stacks to get component configurations.
	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.NoError(t, err, "Failed to process stacks")
	require.NotNil(t, stacks, "Stacks should not be nil")

	// Find the test-component-dev component to verify merge priority.
	comp, stackName, found := findComponentInStacks(stacks, "test-component-dev")
	require.True(t, found, "Component test-component-dev not found in any stack")

	envSection, ok := comp["env"].(map[string]any)
	require.True(t, ok, "Component should have an env section")
	t.Logf("Found test-component-dev in stack %s with env: %v", stackName, envSection)

	// Verify that stack-level env has highest priority.
	assert.Equal(t, "overridden-by-stack", envSection["COMPONENT_ENV_VAR"],
		"Stack env should override component catalog env")
	assert.Equal(t, "from-stack", envSection["TF_OVERRIDE_TEST"],
		"Stack env should override terraform global env")
	assert.Equal(t, "overridden-by-stack", envSection["GLOBAL_OVERRIDE_TEST"],
		"Stack env should override global env")

	// Verify that lower-priority values are still present when not overridden.
	assert.Equal(t, "global-value", envSection["GLOBAL_ENV_VAR"],
		"Global env should be inherited when not overridden")
	assert.Equal(t, "terraform-global", envSection["TF_GLOBAL_VAR"],
		"Terraform global env should be inherited when not overridden")
}

// TestEnvInheritance_ComponentType verifies that Terraform and Helmfile components
// get their respective type-specific env variables merged correctly.
func TestEnvInheritance_ComponentType(t *testing.T) {
	// Set up test fixture path.
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "env-inheritance")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	// Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err, "Failed to initialize CLI config")

	// Process stacks.
	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	require.NoError(t, err, "Failed to process stacks")
	require.NotNil(t, stacks, "Stacks should not be nil")

	// Verify Terraform component gets terraform-specific env variables.
	// Find test-component-dev and verify it has terraform.env variables.
	comp, stackName, found := findComponentInStacks(stacks, "test-component-dev")
	require.True(t, found, "Component test-component-dev not found in any stack")

	envSection, ok := comp["env"].(map[string]any)
	require.True(t, ok, "Component should have an env section")

	val, exists := envSection["TF_GLOBAL_VAR"]
	require.True(t, exists, "Component should have TF_GLOBAL_VAR from terraform.env")
	t.Logf("Component test-component-dev (stack %s) has TF_GLOBAL_VAR: %v", stackName, val)
	assert.Equal(t, "terraform-global", val,
		"Terraform component should inherit terraform.env variables")
}

// TestEnvInheritance_EnvYAMLFunction verifies that the !env YAML function
// reads from the stack manifest's env section first, then falls back to OS environment.
// This test validates the bug fix where !env now checks component env sections
// before checking OS environment variables.
func TestEnvInheritance_EnvYAMLFunction(t *testing.T) {
	// Set up test fixture path.
	fixtureDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "env-inheritance")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	// Load Atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err, "Failed to initialize CLI config")

	// Process stacks with YAML function processing enabled.
	stacks, err := ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, true, true, false, nil, nil)
	require.NoError(t, err, "Failed to process stacks")
	require.NotNil(t, stacks, "Stacks should not be nil")

	// Find the test-component-env-function component.
	comp, stackName, found := findComponentInStacks(stacks, "test-component-env-function")
	require.True(t, found, "Component test-component-env-function not found in any stack")
	t.Logf("Found test-component-env-function in stack %s", stackName)

	// Verify env section has the !env function results.
	envSection, ok := comp["env"].(map[string]any)
	require.True(t, ok, "Component should have an env section")
	t.Logf("Env section: %v", envSection)

	// BAZ should have the value of FOO from globals.yaml via !env.
	bazVal, exists := envSection["BAZ"]
	require.True(t, exists, "BAZ should exist in env section")
	assert.Equal(t, "bar", bazVal,
		"BAZ should have value 'bar' from !env FOO (FOO is defined in globals.yaml)")

	// TF_VAR_FROM_GLOBAL should have the value of TF_GLOBAL_VAR via !env.
	tfVarVal, exists := envSection["TF_VAR_FROM_GLOBAL"]
	require.True(t, exists, "TF_VAR_FROM_GLOBAL should exist in env section")
	assert.Equal(t, "terraform-global", tfVarVal,
		"TF_VAR_FROM_GLOBAL should have value 'terraform-global' from !env TF_GLOBAL_VAR")

	// Verify vars section also processed !env function.
	varsSection, ok := comp["vars"].(map[string]any)
	require.True(t, ok, "Component should have a vars section")
	t.Logf("Vars section: %v", varsSection)

	fooValue, exists := varsSection["foo_value"]
	require.True(t, exists, "foo_value should exist in vars section")
	assert.Equal(t, "bar", fooValue,
		"foo_value should have value 'bar' from !env FOO")
}
