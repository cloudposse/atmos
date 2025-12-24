package extract

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponents(t *testing.T) {
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

	components, err := Components(stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 4) // vpc, eks, ingress, vpc

	// Verify structure of extracted data.
	for _, comp := range components {
		assert.Contains(t, comp, "component")
		assert.Contains(t, comp, "stack")
		assert.Contains(t, comp, "kind") // terraform, helmfile, packer
		assert.Contains(t, comp, "enabled")
		assert.Contains(t, comp, "locked")
		assert.Contains(t, comp, "type") // real, abstract
	}
}

func TestComponents_Nil(t *testing.T) {
	_, err := Components(nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestComponents_EmptyMap(t *testing.T) {
	components, err := Components(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestComponents_InvalidStack(t *testing.T) {
	stacksMap := map[string]any{
		"invalid": "not a map",
	}

	components, err := Components(stacksMap)
	require.NoError(t, err)
	assert.Empty(t, components) // Skips invalid stacks.
}

func TestComponents_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"vars": map[string]any{},
		},
	}

	components, err := Components(stacksMap)
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestComponents_DefaultValues(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{}, // No metadata.
				},
			},
		},
	}

	components, err := Components(stacksMap)
	require.NoError(t, err)
	require.Len(t, components, 1)

	comp := components[0]
	assert.Equal(t, "vpc", comp["component"])
	assert.Equal(t, "test-stack", comp["stack"])
	assert.Equal(t, "terraform", comp["kind"]) // terraform, helmfile, packer
	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["type"]) // real, abstract
}

func TestComponentsForStack(t *testing.T) {
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

	components, err := ComponentsForStack("plat-ue2-dev", stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 2)

	// Verify only dev stack components.
	for _, comp := range components {
		assert.Equal(t, "plat-ue2-dev", comp["stack"])
	}
}

func TestComponentsForStack_NotFound(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{},
	}

	_, err := ComponentsForStack("nonexistent", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestComponentsForStack_InvalidData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid",
	}

	_, err := ComponentsForStack("test", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrParseStacks)
}

func TestComponentsForStack_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}

	_, err := ComponentsForStack("test", stacksMap)
	assert.ErrorIs(t, err, errUtils.ErrParseComponents)
}

func TestComponentsForStack_EmptyComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{},
				"helmfile":  map[string]any{},
			},
		},
	}

	_, err := ComponentsForStack("test", stacksMap)
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
	assert.Equal(t, "terraform", vpc["kind"]) // terraform, helmfile, packer
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
