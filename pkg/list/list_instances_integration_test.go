package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Note: uploadInstances(), processInstances(), and ExecuteListInstancesCmd() are not unit tested here
// because they have hard dependencies on external systems (git operations, config loading, API clients,
// ExecuteDescribeStacks, viper flags). To properly test these functions, they would need refactoring to:
// 1. Accept interface dependencies (GitRepo, ConfigLoader, APIClient) that can be mocked
// 2. Use dependency injection rather than calling global functions directly
// 3. Separate business logic from I/O operations
//
// The current implementation tests below focus on the pure business logic functions that don't
// have external dependencies: processComponentConfig, processComponentType, processStackComponents,
// createInstance, sortInstances, filterProEnabledInstances, and collectInstances.
//
// For integration testing of the full command, see cmd/list_test.go which tests via the CLI.

// Additional edge case tests for existing functions.
func TestProcessComponentConfigEdgeCases(t *testing.T) {
	t.Run("nil component config", func(t *testing.T) {
		result := processComponentConfig("stack1", "comp1", "terraform", nil)
		assert.Nil(t, result)
	})

	t.Run("component config with non-map type", func(t *testing.T) {
		result := processComponentConfig("stack1", "comp1", "terraform", "string")
		assert.Nil(t, result)
	})

	t.Run("component config with slice type", func(t *testing.T) {
		result := processComponentConfig("stack1", "comp1", "terraform", []string{"item1", "item2"})
		assert.Nil(t, result)
	})
}

func TestProcessComponentTypeEdgeCases(t *testing.T) {
	t.Run("nil type components", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", nil)
		assert.Nil(t, result)
	})

	t.Run("type components with non-map type", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", []string{"item1"})
		assert.Nil(t, result)
	})
}

func TestProcessStackComponentsEdgeCases(t *testing.T) {
	t.Run("nil stack config", func(t *testing.T) {
		result := processStackComponents("stack1", nil)
		assert.Nil(t, result)
	})

	t.Run("stack config with non-map type", func(t *testing.T) {
		result := processStackComponents("stack1", "string")
		assert.Nil(t, result)
	})

	t.Run("stack config with slice type", func(t *testing.T) {
		result := processStackComponents("stack1", []string{"item1"})
		assert.Nil(t, result)
	})
}

func TestCreateInstanceEdgeCases(t *testing.T) {
	t.Run("nil component config map", func(t *testing.T) {
		result := createInstance("stack1", "comp1", "terraform", nil)
		// createInstance doesn't check for nil, it creates a instance with empty maps.
		assert.NotNil(t, result)
		assert.Equal(t, "comp1", result.Component)
		assert.Equal(t, "stack1", result.Stack)
	})

	t.Run("component with mixed valid and invalid sections", func(t *testing.T) {
		config := map[string]any{
			"settings": map[string]any{"key": "value"},  // Valid.
			"vars":     "invalid",                       // Invalid.
			"env":      map[string]any{"env": "value"},  // Valid.
			"backend":  []string{"invalid"},             // Invalid.
			"metadata": map[string]any{"meta": "value"}, // Valid.
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		assert.Equal(t, map[string]any{"key": "value"}, result.Settings)
		assert.Empty(t, result.Vars) // Should be empty due to invalid type.
		assert.Equal(t, map[string]any{"env": "value"}, result.Env)
		assert.Empty(t, result.Backend) // Should be empty due to invalid type.
		assert.Equal(t, map[string]any{"meta": "value"}, result.Metadata)
	})

	t.Run("component with abstract type in metadata", func(t *testing.T) {
		config := map[string]any{
			"metadata": map[string]any{"type": "abstract"},
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.Nil(t, result)
	})

	t.Run("component with non-string type in metadata", func(t *testing.T) {
		config := map[string]any{
			"metadata": map[string]any{"type": 123}, // Non-string type.
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result) // Should not be filtered out.
	})
}

func TestSortInstancesEdgeCases(t *testing.T) {
	t.Run("instances with empty component names", func(t *testing.T) {
		instances := []schema.Instance{
			{Component: "", Stack: "stack1"},
			{Component: "vpc", Stack: "stack1"},
			{Component: "", Stack: "stack2"},
		}
		result := sortInstances(instances)
		assert.Len(t, result, 3)
		// The sorting is by stack first, then component.
		// So we should get: stack1 with "", stack1 with "vpc", stack2 with "".
		assert.Equal(t, "", result[0].Component)
		assert.Equal(t, "stack1", result[0].Stack)
		assert.Equal(t, "vpc", result[1].Component)
		assert.Equal(t, "stack1", result[1].Stack)
		assert.Equal(t, "", result[2].Component)
		assert.Equal(t, "stack2", result[2].Stack)
	})

	t.Run("instances with empty stack names", func(t *testing.T) {
		instances := []schema.Instance{
			{Component: "vpc", Stack: ""},
			{Component: "app", Stack: "stack1"},
			{Component: "db", Stack: ""},
		}
		result := sortInstances(instances)
		assert.Len(t, result, 3)
		// Empty strings should sort first.
		assert.Equal(t, "", result[0].Stack)
		assert.Equal(t, "", result[1].Stack)
		assert.Equal(t, "stack1", result[2].Stack)
	})
}

func TestFilterProEnabledInstancesAdditionalEdgeCases(t *testing.T) {
	t.Run("instances with nil settings", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings:  nil,
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})

	t.Run("instances with pro settings but missing drift_detection", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"other": "value",
					},
				},
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})

	t.Run("instances with pro settings but missing enabled key in drift_detection", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"other": "value",
						},
					},
				},
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})

	t.Run("instances with pro settings but drift_detection.enabled is false", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": false,
						},
					},
				},
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})

	t.Run("instances with pro settings and drift_detection.enabled is true", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": true,
						},
					},
				},
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Len(t, filtered, 1)
		assert.Equal(t, "vpc", filtered[0].Component)
		assert.Equal(t, "stack1", filtered[0].Stack)
	})

	t.Run("instances with invalid pro settings structure", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": "invalid", // Not a map.
				},
			},
			{
				Component: "app",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": "invalid", // Not a map.
					},
				},
			},
			{
				Component: "db",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"pro": map[string]interface{}{
						"drift_detection": map[string]interface{}{
							"enabled": "invalid", // Not a bool.
						},
					},
				},
			},
		}

		filtered := filterProEnabledInstances(instances)
		assert.Empty(t, filtered)
	})
}

func TestCollectInstancesEdgeCases(t *testing.T) {
	t.Run("stacks with mixed valid and invalid configs", func(t *testing.T) {
		stacks := map[string]interface{}{
			"stack1": map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc": map[string]interface{}{
							"metadata": map[string]interface{}{"type": "real"},
						},
					},
				},
			},
			"stack2": "invalid", // Invalid stack config.
			"stack3": map[string]interface{}{
				"components": "invalid", // Invalid components.
			},
		}
		result := collectInstances(stacks)
		// Should only process stack1.
		assert.Len(t, result, 1)
		assert.Equal(t, "vpc", result[0].Component)
		assert.Equal(t, "stack1", result[0].Stack)
	})

	t.Run("stacks with empty components", func(t *testing.T) {
		stacks := map[string]interface{}{
			"stack1": map[string]interface{}{
				"components": map[string]interface{}{},
			},
		}
		result := collectInstances(stacks)
		assert.Empty(t, result)
	})
}
