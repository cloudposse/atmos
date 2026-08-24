package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCheckComponentExists_EmptyComponentName tests early return for empty name.
func TestCheckComponentExists_EmptyComponentName(t *testing.T) {
	result := CheckComponentExists(nil, "")
	assert.False(t, result)
}

// TestComponentExistsInStacks_EmptyMap tests empty stacks map.
func TestComponentExistsInStacks_EmptyMap(t *testing.T) {
	result := componentExistsInStacks(map[string]any{}, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_InvalidStackData tests invalid stack data type.
func TestComponentExistsInStacks_InvalidStackData(t *testing.T) {
	stacks := map[string]any{
		"stack1": "invalid-not-a-map",
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_NoComponentsKey tests stack without components key.
func TestComponentExistsInStacks_NoComponentsKey(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"other_key": "value",
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_InvalidComponentsType tests invalid components type.
func TestComponentExistsInStacks_InvalidComponentsType(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"components": "invalid-not-a-map",
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_InvalidComponentTypeMap tests invalid component type map.
func TestComponentExistsInStacks_InvalidComponentTypeMap(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": "invalid-not-a-map",
			},
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_ComponentNotFound tests component not found in stacks.
func TestComponentExistsInStacks_ComponentNotFound(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{"var1": "value1"},
					"eks": map[string]interface{}{"var2": "value2"},
				},
			},
		},
		"stack2": map[string]interface{}{
			"components": map[string]interface{}{
				"helmfile": map[string]interface{}{
					"nginx": map[string]interface{}{"var4": "value4"},
				},
			},
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.False(t, result)
}

// TestComponentExistsInStacks_ComponentFound tests component found in stacks.
func TestComponentExistsInStacks_ComponentFound(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc":            map[string]interface{}{"var1": "value1"},
					"test-component": map[string]interface{}{"var2": "value2"},
				},
			},
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.True(t, result)
}

// TestComponentExistsInStacks_ComponentFoundInSecondStack tests component found in non-first stack.
func TestComponentExistsInStacks_ComponentFoundInSecondStack(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{},
				},
			},
		},
		"stack2": map[string]interface{}{
			"components": map[string]interface{}{
				"helmfile": map[string]interface{}{
					"test-component": map[string]interface{}{},
				},
			},
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.True(t, result)
}

// TestComponentExistsInStacks_MixedValidInvalidStacks tests mixed valid and invalid stack data.
func TestComponentExistsInStacks_MixedValidInvalidStacks(t *testing.T) {
	stacks := map[string]any{
		"invalid-stack": "not-a-map",
		"valid-stack": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"test-component": map[string]interface{}{},
				},
			},
		},
	}
	result := componentExistsInStacks(stacks, "test-component")
	assert.True(t, result)
}
