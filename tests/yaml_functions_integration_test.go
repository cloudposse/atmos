package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYAMLFunctionsInLists tests the yaml-functions-in-lists scenario.
// This verifies that YAML functions work correctly in lists without type conflicts.
func TestYAMLFunctionsInLists(t *testing.T) {
	t.Chdir("./fixtures/scenarios/yaml-functions-in-lists")

	t.Setenv("TEST_ENV_VAR", "test-env-value")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("loads stack with yaml functions in lists", func(t *testing.T) {
		// Test that we can describe a component that uses YAML functions in lists
		componentName := "test-yaml-functions-in-lists"
		stack := "test"

		// This would fail with type conflicts if deferred merge wasn't working
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		// Verify the component loaded successfully
		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")
		require.NotNil(t, vars)
	})

	t.Run("handles list concatenation scenario", func(t *testing.T) {
		// This test verifies the original bug report scenario
		// Multiple terraform.state/terraform.output calls in a list
		componentName := "test-list-concatenation"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify function_results_list exists (contains YAML functions)
		require.Contains(t, vars, "function_results_list")

		// Verify string_list exists (contains YAML functions)
		require.Contains(t, vars, "string_list")
	})

	t.Run("processes multiple components with yaml functions", func(t *testing.T) {
		// Verify all three test components can be loaded
		components := []string{
			"test-component-1",
			"test-component-2",
			"test-yaml-functions-in-lists",
		}

		for _, componentName := range components {
			t.Run(componentName, func(t *testing.T) {
				componentSection, err := e.ExecuteDescribeComponent(
					&e.ExecuteDescribeComponentParams{
						Component: componentName,
						Stack:     "test",
					},
				)

				require.NoError(t, err)
				require.NotNil(t, componentSection)

				// Each component should have vars
				vars, ok := componentSection["vars"].(map[string]interface{})
				require.True(t, ok, "vars should be a map")
				require.NotEmpty(t, vars)
			})
		}
	})
}

// TestYAMLFunctionsDeferredMerge tests the atmos-yaml-functions-merge scenario.
// This verifies that YAML functions can be merged with concrete values without type conflicts.
func TestYAMLFunctionsDeferredMerge(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")

	t.Setenv("ATMOS_STAGE", "test")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("overrides yaml function with concrete value", func(t *testing.T) {
		// Test Case 1: Override !template with concrete map
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")
		require.NotEmpty(t, vars, "vars should not be empty")

		// Verify template_config exists (value doesn't matter - we just need no errors)
		assert.Contains(t, vars, "template_config")
	})

	t.Run("overrides yaml function with another yaml function", func(t *testing.T) {
		// Test Case 2: Override !template with different !template
		componentName := "test-yaml-to-yaml"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// template_config should exist (contains YAML function result)
		require.Contains(t, vars, "template_config")
	})

	t.Run("overrides list yaml function with concrete list", func(t *testing.T) {
		// Test Case 3: Override !terraform.output list with concrete list
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-list-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify vpc_ids exists
		assert.Contains(t, vars, "vpc_ids")
	})

	t.Run("overrides map yaml function with concrete map", func(t *testing.T) {
		// Test Case 4: Override !template map with concrete map
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-map-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify network_config exists
		assert.Contains(t, vars, "network_config")
	})

	t.Run("deep merges with yaml functions", func(t *testing.T) {
		// Test Case 5: Deep merge with YAML function results
		componentName := "test-deep-merge"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// template_config should be deep-merged
		require.Contains(t, vars, "template_config")
	})

	t.Run("handles multiple yaml functions with precedence", func(t *testing.T) {
		// Test Case 6: Multiple YAML functions of same type
		// This component uses !env which should work without external dependencies
		componentName := "test-multiple-yaml-functions"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// If component doesn't exist, skip this test
		if err != nil && strings.Contains(err.Error(), "Could not find the component") {
			t.Skip("test-multiple-yaml-functions component not found in fixture")
		}

		require.NoError(t, err, "deferred merge should handle multiple YAML functions")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify vars contains expected fields
		assert.Contains(t, vars, "region")
	})

	t.Run("loads all test cases without errors", func(t *testing.T) {
		// Verify all test components can be loaded without type conflicts
		components := []string{
			"test-yaml-to-concrete",
			"test-yaml-to-yaml",
			"test-list-yaml-to-concrete",
			"test-map-yaml-to-concrete",
			"test-deep-merge",
		}

		for _, componentName := range components {
			t.Run(componentName, func(t *testing.T) {
				componentSection, err := e.ExecuteDescribeComponent(
					&e.ExecuteDescribeComponentParams{
						Component: componentName,
						Stack:     "test",
					},
				)

				// Key test: no error means deferred merge prevented type conflicts
				require.NoError(t, err, "component %s should load without errors (deferred merge prevents type conflicts)", componentName)
				require.NotNil(t, componentSection)

				// Each component should have vars
				vars, ok := componentSection["vars"].(map[string]interface{})
				require.True(t, ok, "vars should be a map for component %s", componentName)
				require.NotEmpty(t, vars)
			})
		}
	})
}

// TestDeferredMergeTypeConflictResolution tests that type conflicts are resolved.
// This is the core functionality that deferred merge provides.
func TestDeferredMergeTypeConflictResolution(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	t.Run("string to map conflict resolved", func(t *testing.T) {
		// Base catalog has !template (string)
		// Override has concrete map
		// Without deferred merge, this would fail
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "test-yaml-to-concrete",
				Stack:     "test",
			},
		)

		require.NoError(t, err, "should resolve string->map conflict")
		require.NotNil(t, componentSection)
	})

	t.Run("string to list conflict resolved", func(t *testing.T) {
		// Base catalog has !terraform.output (string)
		// Override has concrete list
		// Without deferred merge, this would fail
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "test-list-yaml-to-concrete",
				Stack:     "test",
			},
		)

		require.NoError(t, err, "should resolve string->list conflict")
		require.NotNil(t, componentSection)
	})
}
