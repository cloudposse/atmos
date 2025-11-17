package list

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractStacks(t *testing.T) {
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

	stacks, err := ExtractStacks(stacksMap)
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

func TestExtractStacks_Nil(t *testing.T) {
	_, err := ExtractStacks(nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestExtractStacks_EmptyMap(t *testing.T) {
	stacks, err := ExtractStacks(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

func TestExtractStacksForComponent(t *testing.T) {
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

	stacks, err := ExtractStacksForComponent("vpc", stacksMap)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)

	// Verify only stacks with vpc component.
	for _, stack := range stacks {
		assert.Equal(t, "vpc", stack["component"])
		stackName := stack["stack"].(string)
		assert.True(t, stackName == "plat-ue2-dev" || stackName == "plat-ue2-prod")
	}
}

func TestExtractStacksForComponent_MultipleTypes(t *testing.T) {
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

	stacks, err := ExtractStacksForComponent("ingress", stacksMap)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)

	// Verify both stacks with ingress helmfile component.
	for _, stack := range stacks {
		assert.Equal(t, "ingress", stack["component"])
	}
}

func TestExtractStacksForComponent_NotFound(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
	}

	_, err := ExtractStacksForComponent("nonexistent", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestExtractStacksForComponent_Nil(t *testing.T) {
	_, err := ExtractStacksForComponent("vpc", nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestExtractStacksForComponent_InvalidData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid",
	}

	_, err := ExtractStacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestExtractStacksForComponent_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}

	_, err := ExtractStacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestExtractStacksForComponent_EmptyComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{},
				"helmfile":  map[string]any{},
			},
		},
	}

	_, err := ExtractStacksForComponent("vpc", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}
