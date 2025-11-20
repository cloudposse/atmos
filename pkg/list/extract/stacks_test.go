package extract

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStacks(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
		"plat-ue2-prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
		"plat-uw2-staging": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"eks": map[string]any{},
				},
			},
		},
	}

	stacks, err := Stacks(stacksMap)
	require.NoError(t, err)
	assert.Len(t, stacks, 3)

	// Verify structure of extracted data.
	stackNames := make(map[string]bool)
	for _, stack := range stacks {
		assert.Contains(t, stack, "stack")
		stackName, ok := stack["stack"].(string)
		require.True(t, ok)
		stackNames[stackName] = true
	}

	// Verify all stacks are present.
	assert.True(t, stackNames["plat-ue2-dev"])
	assert.True(t, stackNames["plat-ue2-prod"])
	assert.True(t, stackNames["plat-uw2-staging"])
}

func TestStacks_Nil(t *testing.T) {
	_, err := Stacks(nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestStacks_EmptyMap(t *testing.T) {
	stacks, err := Stacks(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

func TestStacksForComponent(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
					"eks": map[string]any{},
				},
			},
		},
		"plat-ue2-prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
					"rds": map[string]any{},
				},
			},
		},
		"plat-uw2-staging": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"eks": map[string]any{},
				},
			},
		},
	}

	stacks, err := StacksForComponent("vpc", stacksMap)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)

	// Verify only stacks with vpc component.
	for _, stack := range stacks {
		assert.Equal(t, "vpc", stack["component"])
		stackName := stack["stack"].(string)
		assert.True(t, stackName == "plat-ue2-dev" || stackName == "plat-ue2-prod")
	}
}

func TestStacksForComponent_MultipleTypes(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{},
				},
			},
		},
		"plat-ue2-prod": map[string]any{
			"components": map[string]any{
				"helmfile": map[string]any{
					"ingress": map[string]any{},
				},
			},
		},
	}

	stacks, err := StacksForComponent("ingress", stacksMap)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)

	// Verify both stacks with ingress helmfile component.
	for _, stack := range stacks {
		assert.Equal(t, "ingress", stack["component"])
	}
}

func TestStacksForComponent_NotFound(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
	}

	_, err := StacksForComponent("nonexistent", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_Nil(t *testing.T) {
	_, err := StacksForComponent("vpc", nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestStacksForComponent_InvalidData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid",
	}

	_, err := StacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}

	_, err := StacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_EmptyComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{},
				"helmfile":  map[string]any{},
			},
		},
	}

	_, err := StacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}
