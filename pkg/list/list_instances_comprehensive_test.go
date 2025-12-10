package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Mock interfaces for testing.

// MockAtmosProAPIClientInterface is a mock implementation of the Atmos Pro API client interface for testing purposes.
type MockAtmosProAPIClientInterface struct {
	mock.Mock
}

func (m *MockAtmosProAPIClientInterface) UploadInstances(req *dtos.InstancesUploadRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockAtmosProAPIClientInterface) UploadInstanceStatus(req *dtos.InstanceStatusUploadRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

// Test processComponentConfig edge cases.
func TestProcessComponentConfig(t *testing.T) {
	t.Run("invalid component config type", func(t *testing.T) {
		result := processComponentConfig("stack1", "comp1", "terraform", "invalid")
		assert.Nil(t, result)
	})

	t.Run("valid component config", func(t *testing.T) {
		config := map[string]any{
			"settings": map[string]any{"pro": map[string]any{"drift_detection": map[string]any{"enabled": true}}},
			"vars":     map[string]any{"key": "value"},
		}
		result := processComponentConfig("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		assert.Equal(t, "comp1", result.Component)
		assert.Equal(t, "stack1", result.Stack)
		assert.Equal(t, "terraform", result.ComponentType)
	})
}

// Test processComponentType edge cases.
func TestProcessComponentType(t *testing.T) {
	t.Run("invalid type components", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", "invalid")
		assert.Nil(t, result)
	})

	t.Run("empty type components", func(t *testing.T) {
		result := processComponentType("stack1", "terraform", map[string]any{})
		assert.Empty(t, result)
	})
}

// Test processStackComponents edge cases.
func TestProcessStackComponents(t *testing.T) {
	t.Run("invalid stack config", func(t *testing.T) {
		result := processStackComponents("stack1", "invalid")
		assert.Nil(t, result)
	})

	t.Run("stack config without components", func(t *testing.T) {
		config := map[string]any{"other": "value"}
		result := processStackComponents("stack1", config)
		assert.Nil(t, result)
	})

	t.Run("invalid components type", func(t *testing.T) {
		config := map[string]any{"components": "invalid"}
		result := processStackComponents("stack1", config)
		assert.Nil(t, result)
	})
}

// Test createInstance edge cases.
func TestCreateInstance(t *testing.T) {
	t.Run("abstract component should be filtered", func(t *testing.T) {
		config := map[string]any{
			"metadata": map[string]any{"type": "abstract"},
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.Nil(t, result)
	})

	t.Run("component with all sections", func(t *testing.T) {
		config := map[string]any{
			"settings": map[string]any{"key": "value"},
			"vars":     map[string]any{"var": "value"},
			"env":      map[string]any{"env": "value"},
			"backend":  map[string]any{"backend": "value"},
			"metadata": map[string]any{"meta": "value"},
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		assert.Equal(t, "comp1", result.Component)
		assert.Equal(t, "stack1", result.Stack)
		assert.Equal(t, "terraform", result.ComponentType)
		assert.Equal(t, map[string]any{"key": "value"}, result.Settings)
		assert.Equal(t, map[string]any{"var": "value"}, result.Vars)
		assert.Equal(t, map[string]any{"env": "value"}, result.Env)
		assert.Equal(t, map[string]any{"backend": "value"}, result.Backend)
		assert.Equal(t, map[string]any{"meta": "value"}, result.Metadata)
	})

	t.Run("component with invalid section types", func(t *testing.T) {
		config := map[string]any{
			"settings": "invalid",
			"vars":     "invalid",
			"env":      "invalid",
			"backend":  "invalid",
			"metadata": "invalid",
		}
		result := createInstance("stack1", "comp1", "terraform", config)
		assert.NotNil(t, result)
		// Should have empty maps for invalid sections.
		assert.Empty(t, result.Settings)
		assert.Empty(t, result.Vars)
		assert.Empty(t, result.Env)
		assert.Empty(t, result.Backend)
		assert.Empty(t, result.Metadata)
	})
}

// Test sortInstances.
func TestSortInstances(t *testing.T) {
	t.Run("empty instances", func(t *testing.T) {
		result := sortInstances([]schema.Instance{})
		assert.Empty(t, result)
	})

	t.Run("single instance", func(t *testing.T) {
		instances := []schema.Instance{
			{Component: "vpc", Stack: "stack1"},
		}
		result := sortInstances(instances)
		assert.Len(t, result, 1)
		assert.Equal(t, "vpc", result[0].Component)
	})

	t.Run("multiple instances with same stack", func(t *testing.T) {
		instances := []schema.Instance{
			{Component: "db", Stack: "stack1"},
			{Component: "vpc", Stack: "stack1"},
			{Component: "app", Stack: "stack1"},
		}
		result := sortInstances(instances)
		assert.Len(t, result, 3)
		// Should be sorted by component name.
		assert.Equal(t, "app", result[0].Component)
		assert.Equal(t, "db", result[1].Component)
		assert.Equal(t, "vpc", result[2].Component)
	})

	t.Run("multiple instances with different stacks", func(t *testing.T) {
		instances := []schema.Instance{
			{Component: "vpc", Stack: "stack2"},
			{Component: "app", Stack: "stack1"},
			{Component: "db", Stack: "stack1"},
		}
		result := sortInstances(instances)
		assert.Len(t, result, 3)
		// Should be sorted by stack first, then component.
		assert.Equal(t, "app", result[0].Component)
		assert.Equal(t, "stack1", result[0].Stack)
		assert.Equal(t, "db", result[1].Component)
		assert.Equal(t, "stack1", result[1].Stack)
		assert.Equal(t, "vpc", result[2].Component)
		assert.Equal(t, "stack2", result[2].Stack)
	})
}

// Test filterProEnabledInstances edge cases.
func TestFilterProEnabledInstancesEdgeCases(t *testing.T) {
	t.Run("instances with invalid pro settings", func(t *testing.T) {
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

	t.Run("instances with missing pro settings", func(t *testing.T) {
		instances := []schema.Instance{
			{
				Component: "vpc",
				Stack:     "stack1",
				Settings:  map[string]interface{}{},
			},
			{
				Component: "app",
				Stack:     "stack1",
				Settings: map[string]interface{}{
					"other": "value",
				},
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
}

// Test collectInstances edge cases.
func TestCollectInstances(t *testing.T) {
	t.Run("empty stacks map", func(t *testing.T) {
		result := collectInstances(map[string]interface{}{})
		assert.Empty(t, result)
	})

	t.Run("stacks with invalid stack configs", func(t *testing.T) {
		stacks := map[string]interface{}{
			"stack1": "invalid",
			"stack2": map[string]interface{}{
				"components": "invalid",
			},
		}
		result := collectInstances(stacks)
		assert.Empty(t, result)
	})
}
