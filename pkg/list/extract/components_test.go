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
		assert.Contains(t, comp, "type") // terraform, helmfile, packer
		assert.Contains(t, comp, "enabled")
		assert.Contains(t, comp, "locked")
		assert.Contains(t, comp, "component_type") // real, abstract
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
	assert.Equal(t, "terraform", comp["type"]) // terraform, helmfile, packer
	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"]) // real, abstract
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
	assert.Equal(t, "terraform", vpc["type"]) // terraform, helmfile, packer
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

// Tests for UniqueComponents function.
func TestUniqueComponents(t *testing.T) {
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"enabled":   true,
							"locked":    false,
							"type":      "real",
							"component": "vpc-base",
						},
					},
					"eks": map[string]any{
						"metadata": map[string]any{
							"enabled": true,
						},
					},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{},
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
					"rds": map[string]any{},
				},
			},
		},
	}

	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	// Should have 4 unique components: vpc, eks, ingress, rds.
	assert.Len(t, components, 4)

	// Verify vpc component has stack_count of 2.
	for _, comp := range components {
		if comp["component"] == "vpc" && comp["type"] == "terraform" {
			assert.Equal(t, 2, comp["stack_count"], "vpc should appear in 2 stacks")
		}
	}
}

func TestUniqueComponents_Nil(t *testing.T) {
	_, err := UniqueComponents(nil, "")
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestUniqueComponents_WithStackFilter(t *testing.T) {
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
					"rds": map[string]any{},
				},
			},
		},
		"other-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"s3": map[string]any{},
				},
			},
		},
	}

	// Filter to only plat-* stacks.
	components, err := UniqueComponents(stacksMap, "plat-*")
	require.NoError(t, err)
	// Should only include vpc and rds from plat-* stacks.
	assert.Len(t, components, 2)

	componentNames := make(map[string]bool)
	for _, comp := range components {
		componentNames[comp["component"].(string)] = true
	}
	assert.True(t, componentNames["vpc"])
	assert.True(t, componentNames["rds"])
	assert.False(t, componentNames["s3"])
}

func TestUniqueComponents_InvalidStackPattern(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
	}

	// Invalid pattern with unmatched bracket.
	components, err := UniqueComponents(stacksMap, "[invalid")
	require.NoError(t, err)
	// Should skip the stack due to pattern error.
	assert.Empty(t, components)
}

func TestUniqueComponents_EmptyStacks(t *testing.T) {
	stacksMap := map[string]any{}
	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestUniqueComponents_InvalidStackData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid-not-a-map",
	}
	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestUniqueComponents_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}
	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestUniqueComponents_AllTypes(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{},
				},
				"packer": map[string]any{
					"ami-builder": map[string]any{},
				},
			},
		},
	}

	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	assert.Len(t, components, 3)

	// Verify all types are present.
	types := make(map[string]bool)
	for _, comp := range components {
		types[comp["type"].(string)] = true
	}
	assert.True(t, types["terraform"])
	assert.True(t, types["helmfile"])
	assert.True(t, types["packer"])
}

// Tests for enrichComponentWithMetadata edge cases.
func TestEnrichComponentWithMetadata_WithVars(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
		"stack":     "test-stack",
		"type":      "terraform",
	}

	componentData := map[string]any{
		"vars": map[string]any{
			"region":      "us-east-1",
			"environment": "prod",
		},
	}

	enrichComponentWithMetadata(comp, componentData)

	vars, ok := comp["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "us-east-1", vars["region"])
	assert.Equal(t, "prod", vars["environment"])
}

func TestEnrichComponentWithMetadata_WithSettings(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
	}

	componentData := map[string]any{
		"settings": map[string]any{
			"spacelift": map[string]any{
				"workspace_enabled": true,
			},
		},
	}

	enrichComponentWithMetadata(comp, componentData)

	settings, ok := comp["settings"].(map[string]any)
	require.True(t, ok)
	assert.NotNil(t, settings["spacelift"])
}

func TestEnrichComponentWithMetadata_WithComponentFolder(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
	}

	componentData := map[string]any{
		"component_folder": "infra/vpc",
	}

	enrichComponentWithMetadata(comp, componentData)

	assert.Equal(t, "infra/vpc", comp["component_folder"])
}

func TestEnrichComponentWithMetadata_WithTerraformComponent(t *testing.T) {
	comp := map[string]any{
		"component": "vpc-dev",
	}

	componentData := map[string]any{
		"terraform_component": "vpc",
	}

	enrichComponentWithMetadata(comp, componentData)

	assert.Equal(t, "vpc", comp["terraform_component"])
}

func TestEnrichComponentWithMetadata_InvalidData(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
	}

	// Pass non-map data.
	enrichComponentWithMetadata(comp, "invalid")

	// Component should remain unchanged (just default fields set).
	assert.Equal(t, "vpc", comp["component"])
}

func TestEnrichComponentWithMetadata_WithMetadataComponent(t *testing.T) {
	comp := map[string]any{
		"component": "vpc-derived",
	}

	componentData := map[string]any{
		"metadata": map[string]any{
			"component": "vpc",
			"enabled":   true,
			"locked":    false,
			"type":      "abstract",
		},
	}

	enrichComponentWithMetadata(comp, componentData)

	// component_folder should be set from metadata.component.
	assert.Equal(t, "vpc", comp["component_folder"])
	assert.Equal(t, "abstract", comp["component_type"])
}

func TestEnrichComponentWithMetadata_ComponentFolderFallback(t *testing.T) {
	comp := map[string]any{
		"component": "my-vpc",
	}

	componentData := map[string]any{
		"metadata": map[string]any{
			"enabled": true,
		},
	}

	enrichComponentWithMetadata(comp, componentData)

	// component_folder should fall back to component name.
	assert.Equal(t, "my-vpc", comp["component_folder"])
}

// Tests for extractMetadataFields function.
func TestExtractMetadataFields(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
	}

	metadata := map[string]any{
		"enabled":   false,
		"locked":    true,
		"type":      "abstract",
		"component": "base-vpc",
	}

	extractMetadataFields(comp, metadata)

	assert.Equal(t, false, comp["enabled"])
	assert.Equal(t, true, comp["locked"])
	assert.Equal(t, "abstract", comp["component_type"])
	assert.Equal(t, "base-vpc", comp["component_folder"])
	assert.NotEmpty(t, comp["status"])
	assert.Equal(t, "locked", comp["status_text"])
}

// Tests for setDefaultMetadataFields function.
func TestSetDefaultMetadataFields(t *testing.T) {
	comp := map[string]any{
		"component": "vpc",
	}

	setDefaultMetadataFields(comp)

	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"])
	assert.NotEmpty(t, comp["status"])
	assert.Equal(t, "enabled", comp["status_text"])
	assert.Equal(t, "vpc", comp["component_folder"])
}

// Tests for helper functions.
func TestGetBoolWithDefault(t *testing.T) {
	m := map[string]any{
		"enabled": true,
		"locked":  false,
	}

	assert.True(t, getBoolWithDefault(m, "enabled", false))
	assert.False(t, getBoolWithDefault(m, "locked", true))
	assert.True(t, getBoolWithDefault(m, "missing", true))
	assert.False(t, getBoolWithDefault(m, "missing", false))
}

func TestGetStringWithDefault(t *testing.T) {
	m := map[string]any{
		"type":   "abstract",
		"region": "us-east-1",
	}

	assert.Equal(t, "abstract", getStringWithDefault(m, "type", "real"))
	assert.Equal(t, "us-east-1", getStringWithDefault(m, "region", ""))
	assert.Equal(t, "default", getStringWithDefault(m, "missing", "default"))
}

// Tests for enrichUniqueComponentMetadata function.
func TestEnrichUniqueComponentMetadata_WithMetadata(t *testing.T) {
	comp := map[string]any{
		"component":   "vpc",
		"type":        "terraform",
		"stack_count": 0,
	}

	componentData := map[string]any{
		"metadata": map[string]any{
			"enabled":   true,
			"locked":    false,
			"type":      "real",
			"component": "base-vpc",
		},
		"component_folder": "infra/vpc",
	}

	enrichUniqueComponentMetadata(comp, componentData)

	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"])
	assert.Equal(t, "infra/vpc", comp["component_folder"])
}

func TestEnrichUniqueComponentMetadata_WithoutMetadata(t *testing.T) {
	comp := map[string]any{
		"component":   "vpc",
		"type":        "terraform",
		"stack_count": 0,
	}

	componentData := map[string]any{
		"vars": map[string]any{
			"region": "us-east-1",
		},
	}

	enrichUniqueComponentMetadata(comp, componentData)

	// Should use defaults.
	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"])
}

func TestEnrichUniqueComponentMetadata_InvalidData(t *testing.T) {
	comp := map[string]any{
		"component":   "vpc",
		"type":        "terraform",
		"stack_count": 0,
	}

	enrichUniqueComponentMetadata(comp, "invalid")

	// Should use defaults.
	assert.Equal(t, true, comp["enabled"])
	assert.Equal(t, false, comp["locked"])
	assert.Equal(t, "real", comp["component_type"])
}

// Tests for buildBaseComponent function.
func TestBuildBaseComponent(t *testing.T) {
	comp := buildBaseComponent("vpc", "test-stack", "terraform")

	assert.Equal(t, "vpc", comp["component"])
	assert.Equal(t, "test-stack", comp["stack"])
	assert.Equal(t, "terraform", comp["type"])
}

// TestUniqueComponents_EnabledFieldDeterministic_Issue2359 reproduces
// https://github.com/cloudposse/atmos/issues/2359. When the same component
// has different `enabled` values across stacks, UniqueComponents must
// return a deterministic `enabled` value. Currently it records metadata
// only from the first stack iterated (in extractUniqueComponentType, see
// components.go:251-271), and Go map iteration order is randomized — so
// the result varies between runs. This causes `atmos list components
// --enabled=false` to return different content on every invocation.
func TestUniqueComponents_EnabledFieldDeterministic_Issue2359(t *testing.T) {
	// vpc is disabled in two stacks and enabled in a third.
	// Whichever stack iterates first determines the reported `enabled`.
	stacksMap := map[string]any{
		"stack-a-disabled": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{"enabled": false},
					},
				},
			},
		},
		"stack-b-disabled": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{"enabled": false},
					},
				},
			},
		},
		"stack-c-enabled": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{"enabled": true},
					},
				},
			},
		},
	}

	const iterations = 200
	seenEnabled := make(map[bool]int)
	for i := 0; i < iterations; i++ {
		components, err := UniqueComponents(stacksMap, "")
		require.NoError(t, err)
		require.Len(t, components, 1)
		enabled, ok := components[0]["enabled"].(bool)
		require.True(t, ok, "enabled field missing or not bool")
		seenEnabled[enabled]++
	}

	// With the pre-fix code, both true and false appeared across iterations
	// because Go map iteration order is randomized. Post-fix, the value must
	// be deterministic (only one key in the map).
	assert.Lenf(t, seenEnabled, 1,
		"issue #2359: enabled field is non-deterministic across runs — "+
			"saw distribution %v over %d iterations",
		seenEnabled, iterations)

	// Fix policy: any-disabled-wins. vpc is disabled in at least one stack,
	// so the aggregate must be disabled regardless of iteration order.
	assert.Equal(t, 200, seenEnabled[false],
		"issue #2359: when any instance is disabled, the unique component "+
			"must be reported as disabled (any-disabled-wins)")
}

// TestUniqueComponents_AnyDisabledWins verifies the aggregation policy:
// when a component is enabled in some stacks and disabled in others, the
// unique component is reported as disabled so it appears under
// `atmos list components --enabled=false`.
func TestUniqueComponents_AnyDisabledWins(t *testing.T) {
	stacksMap := map[string]any{
		"stack-1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{"enabled": true},
					},
					"eks": map[string]any{
						"metadata": map[string]any{"enabled": true, "locked": true},
					},
				},
			},
		},
		"stack-2": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{"enabled": false},
					},
					"eks": map[string]any{
						"metadata": map[string]any{"enabled": true, "locked": false},
					},
				},
			},
		},
	}

	components, err := UniqueComponents(stacksMap, "")
	require.NoError(t, err)
	require.Len(t, components, 2)

	byName := make(map[string]map[string]any, len(components))
	for _, c := range components {
		byName[c["component"].(string)] = c
	}

	// vpc: enabled in stack-1, disabled in stack-2 → aggregate disabled.
	require.Contains(t, byName, "vpc")
	assert.Equal(t, false, byName["vpc"]["enabled"])
	assert.Equal(t, "disabled", byName["vpc"]["status_text"])
	assert.Equal(t, 2, byName["vpc"]["stack_count"])

	// eks: enabled in both; locked in stack-1, unlocked in stack-2 → aggregate locked.
	require.Contains(t, byName, "eks")
	assert.Equal(t, true, byName["eks"]["enabled"])
	assert.Equal(t, true, byName["eks"]["locked"])
	assert.Equal(t, "locked", byName["eks"]["status_text"])
	assert.Equal(t, 2, byName["eks"]["stack_count"])
}

// TestUniqueComponents_DeterministicOrder verifies that the output slice
// itself is returned in a stable order across runs, not just that the
// per-component metadata is stable.
func TestUniqueComponents_DeterministicOrder(t *testing.T) {
	stacksMap := map[string]any{
		"stack-a": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"zebra": map[string]any{},
					"alpha": map[string]any{},
					"mango": map[string]any{},
				},
				"helmfile": map[string]any{
					"alpha": map[string]any{},
				},
			},
		},
		"stack-b": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"alpha": map[string]any{},
				},
			},
		},
	}

	var first []string
	const iterations = 50
	for i := 0; i < iterations; i++ {
		components, err := UniqueComponents(stacksMap, "")
		require.NoError(t, err)
		names := make([]string, len(components))
		for j, c := range components {
			names[j] = c["component"].(string) + ":" + c["type"].(string)
		}
		if i == 0 {
			first = names
			continue
		}
		require.Equal(t, first, names,
			"issue #2359: UniqueComponents output order must be deterministic")
	}
}

// Test extractUniqueComponentType function.
func TestExtractUniqueComponentType(t *testing.T) {
	seen := make(map[string]map[string]any)
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

	extractUniqueComponentType("terraform", componentsMap, seen)

	assert.Len(t, seen, 2)
	assert.Contains(t, seen, "vpc:terraform")
	assert.Contains(t, seen, "eks:terraform")
	assert.Equal(t, 1, seen["vpc:terraform"]["stack_count"])
	assert.Equal(t, 1, seen["eks:terraform"]["stack_count"])
}

func TestExtractUniqueComponentType_MultipleStacks(t *testing.T) {
	seen := make(map[string]map[string]any)

	// First stack.
	componentsMap1 := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{},
		},
	}
	extractUniqueComponentType("terraform", componentsMap1, seen)

	// Second stack (same component).
	componentsMap2 := map[string]any{
		"terraform": map[string]any{
			"vpc": map[string]any{},
		},
	}
	extractUniqueComponentType("terraform", componentsMap2, seen)

	assert.Len(t, seen, 1)
	assert.Equal(t, 2, seen["vpc:terraform"]["stack_count"])
}

func TestExtractUniqueComponentType_InvalidType(t *testing.T) {
	seen := make(map[string]map[string]any)
	componentsMap := map[string]any{
		"terraform": "not-a-map",
	}

	extractUniqueComponentType("terraform", componentsMap, seen)

	assert.Empty(t, seen)
}

func TestExtractUniqueComponentType_MissingType(t *testing.T) {
	seen := make(map[string]map[string]any)
	componentsMap := map[string]any{
		"helmfile": map[string]any{},
	}

	extractUniqueComponentType("terraform", componentsMap, seen)

	assert.Empty(t, seen)
}

// Test Components with all component types.
func TestComponents_AllTypes(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{},
				},
				"packer": map[string]any{
					"ami": map[string]any{},
				},
			},
		},
	}

	components, err := Components(stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 3)

	types := make(map[string]bool)
	for _, comp := range components {
		types[comp["type"].(string)] = true
	}
	assert.True(t, types["terraform"])
	assert.True(t, types["helmfile"])
	assert.True(t, types["packer"])
}

// Test ComponentsForStack with all component types.
func TestComponentsForStack_AllTypes(t *testing.T) {
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
				"helmfile": map[string]any{
					"ingress": map[string]any{},
				},
				"packer": map[string]any{
					"ami": map[string]any{},
				},
			},
		},
	}

	components, err := ComponentsForStack("test-stack", stacksMap)
	require.NoError(t, err)
	assert.Len(t, components, 3)
}
