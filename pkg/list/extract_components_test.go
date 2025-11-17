package list

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractComponents(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
							"locked":  false,
							"type":    "real",
						},
					},
					"eks": map[string]any{
						"metadata": map[string]any{
							"enabled": false,
							"locked":  true,
							"type":    "real",
						},
					},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
							"type":    "abstract",
						},
					},
				},
			},
		},
		"plat-ue2-prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	components, err := ExtractComponents(stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 4) // vpc, eks, ingress, vpc

	// Verify structure of extracted data.
	for _, comp := range components {
		assert.Contains(t, comp, "component")
		assert.Contains(t, comp, "stack")
		assert.Contains(t, comp, "type")
		assert.Contains(t, comp, "enabled")
		assert.Contains(t, comp, "locked")
		assert.Contains(t, comp, "component_type")
	}
}

func TestExtractComponents_Nil(t *testing.T) {
	_, err := ExtractComponents(nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestExtractComponents_EmptyMap(t *testing.T) {
	components, err := ExtractComponents(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestExtractComponents_InvalidStack(t *testing.T) {
	stacksMap := map[string]any{
		"invalid": "not a map",
	}

	components, err := ExtractComponents(stacksMap)
	require.NoError(t, err)
	assert.Empty(t, components) // Skips invalid stacks.
}

func TestExtractComponents_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"vars": map[string]any{},
		},
	}

	components, err := ExtractComponents(stacksMap)
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestExtractComponents_DefaultValues(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{}, // No metadata.
				},
			},
		},
	}

	components, err := ExtractComponents(stacksMap)
	require.NoError(t, err)
	require.Len(t, components, 1)

	comp := components[0]
	assert.Equal(t, "vpc", comp["component"])
	assert.Equal(t, "test-stack", comp["stack"])
	assert.Equal(t, "terraform", comp["type"])
	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"])
}

func TestExtractComponentsForStack(t *testing.T) {
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
					"rds": map[string]any{},
				},
			},
		},
	}

	components, err := ExtractComponentsForStack("plat-ue2-dev", stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 2)

	// Verify only dev stack components.
	for _, comp := range components {
		assert.Equal(t, "plat-ue2-dev", comp["stack"])
	}
}

func TestExtractComponentsForStack_NotFound(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{},
	}

	_, err := ExtractComponentsForStack("nonexistent", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestExtractComponentsForStack_InvalidData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid",
	}

	_, err := ExtractComponentsForStack("test", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrParseStacks)
}

func TestExtractComponentsForStack_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}

	_, err := ExtractComponentsForStack("test", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrParseComponents)
}

func TestExtractComponentsForStack_EmptyComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{},
				"helmfile":  map[string]any{},
			},
		},
	}

	_, err := ExtractComponentsForStack("test", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrNoComponentsFound)
}

func TestExtractComponentType(t *testing.T) {
	componentsMap := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{
				"metadata": map[string]any{
					"enabled": true,
				},
			},
			"eks": map[string]any{},
		},
	}

	components := extractComponentType("test-stack", "terraform", componentsMap)
	assert.Len(t, components, 2)

	// Find vpc component.
	var vpc map[string]any
	for _, comp := range components {
		if comp["component"] == "vpc" {
			vpc = comp
			break
		}
	}

	require.NotNil(t, vpc)
	assert.Equal(t, "vpc", vpc["component"])
	assert.Equal(t, "test-stack", vpc["stack"])
	assert.Equal(t, "terraform", vpc["type"])
	assert.Equal(t, true, vpc["enabled"])
}

func TestExtractComponentType_InvalidType(t *testing.T) {
	componentsMap := map[string]any{
		"terraform": "not a map",
	}

	components := extractComponentType("test-stack", "terraform", componentsMap)
	assert.Nil(t, components)
}

func TestExtractComponentType_MissingType(t *testing.T) {
	componentsMap := map[string]any{
		"helmfile": map[string]any{},
	}

	components := extractComponentType("test-stack", "terraform", componentsMap)
	assert.Nil(t, components)
}
