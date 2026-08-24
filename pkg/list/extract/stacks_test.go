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

	stacks, err := Stacks(stacksMap, nil, nil)
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
	_, err := Stacks(nil, nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestStacks_EmptyMap(t *testing.T) {
	stacks, err := Stacks(map[string]any{}, nil, nil)
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

	stacks, err := StacksForComponent("vpc", stacksMap, nil, nil)
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

	stacks, err := StacksForComponent("ingress", stacksMap, nil, nil)
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

	_, err := StacksForComponent("nonexistent", stacksMap, nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_Nil(t *testing.T) {
	_, err := StacksForComponent("vpc", nil, nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestStacksForComponent_InvalidData(t *testing.T) {
	stacksMap := map[string]any{
		"test": "invalid",
	}

	_, err := StacksForComponent("vpc", stacksMap, nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_NoComponents(t *testing.T) {
	stacksMap := map[string]any{
		"test": map[string]any{
			"vars": map[string]any{},
		},
	}

	_, err := StacksForComponent("vpc", stacksMap, nil, nil)
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

	_, err := StacksForComponent("vpc", stacksMap, nil, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacks_ExtractsVars(t *testing.T) {
	// This tests that vars are extracted from components and exposed for template access.
	// The structure mirrors ExecuteDescribeStacks output where vars are nested
	// inside components: stackMap["components"]["terraform"]["<component>"]["vars"].
	// Templates access vars via {{ .vars.fieldname }}.
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"namespace":   "acme",
							"tenant":      "plat",
							"environment": "ue2",
							"stage":       "dev",
							"region":      "us-east-2",
						},
					},
				},
			},
		},
	}

	stacks, err := Stacks(stacksMap, nil, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)

	stack := stacks[0]
	assert.Equal(t, "plat-ue2-dev", stack["stack"])

	// Vars are exposed for template access (e.g., {{ .vars.namespace }}).
	vars, ok := stack["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", vars["namespace"])
	assert.Equal(t, "plat", vars["tenant"])
	assert.Equal(t, "ue2", vars["environment"])
	assert.Equal(t, "dev", vars["stage"])
	assert.Equal(t, "us-east-2", vars["region"])
}

func TestStacks_NoVars(t *testing.T) {
	// When components have no vars, an empty vars map should be set.
	stacksMap := map[string]any{
		"test-stack": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
	}

	stacks, err := Stacks(stacksMap, nil, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)

	stack := stacks[0]
	assert.Equal(t, "test-stack", stack["stack"])

	// Vars should be an empty map when not found.
	vars, ok := stack["vars"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, vars)
}

// tagsLabelsStacksMap builds a two-stack fixture where, in "plat-ue2-dev",
// component "vpc" carries only a tag and component "eks" carries only a
// label — so a combined tags+labels filter must NOT match via a union of
// different components. "plat-ue2-prod" has an unmatched component.
func tagsLabelsStacksMap() map[string]any {
	return map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"tags": []any{"network"},
						},
					},
					"eks": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{"team": "platform"},
						},
					},
				},
			},
		},
		"plat-ue2-prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"rds": map[string]any{
						"metadata": map[string]any{
							"tags": []any{"database"},
						},
					},
				},
			},
		},
	}
}

func stackNamesOf(stacks []map[string]any) []string {
	names := make([]string, 0, len(stacks))
	for _, s := range stacks {
		names = append(names, s["stack"].(string))
	}
	return names
}

func TestStacks_TagsFilter(t *testing.T) {
	stacks, err := Stacks(tagsLabelsStacksMap(), []string{"network"}, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)
	assert.Equal(t, "plat-ue2-dev", stacks[0]["stack"])
}

func TestStacks_LabelsFilter(t *testing.T) {
	stacks, err := Stacks(tagsLabelsStacksMap(), nil, map[string]string{"team": "platform"})
	require.NoError(t, err)
	require.Len(t, stacks, 1)
	assert.Equal(t, "plat-ue2-dev", stacks[0]["stack"])
}

func TestStacks_TagsAnyMatchAcrossStacks(t *testing.T) {
	// Any-match tags: either tag qualifies its stack.
	stacks, err := Stacks(tagsLabelsStacksMap(), []string{"network", "database"}, nil)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)
	assert.ElementsMatch(t, []string{"plat-ue2-dev", "plat-ue2-prod"}, stackNamesOf(stacks))
}

func TestStacks_NoCrossComponentUnionLeakage(t *testing.T) {
	// The tag lives on "vpc" and the label on "eks": no single component
	// satisfies both filters, so the stack must be excluded even though each
	// condition is satisfied somewhere in the stack.
	stacks, err := Stacks(tagsLabelsStacksMap(), []string{"network"}, map[string]string{"team": "platform"})
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

func TestStacks_FilterMatchesNothing(t *testing.T) {
	stacks, err := Stacks(tagsLabelsStacksMap(), []string{"nonexistent"}, nil)
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

func TestStacks_EmptyFiltersPreserveAllStacks(t *testing.T) {
	stacks, err := Stacks(tagsLabelsStacksMap(), nil, nil)
	require.NoError(t, err)
	assert.Len(t, stacks, 2)
}

func TestStacks_InvalidStackDataExcludedByFilter(t *testing.T) {
	stacksMap := map[string]any{
		"broken": "not-a-map",
	}

	// Without filters the malformed stack is still listed (legacy behavior)...
	stacks, err := Stacks(stacksMap, nil, nil)
	require.NoError(t, err)
	assert.Len(t, stacks, 1)

	// ...but an active filter can never match a stack without component data.
	stacks, err = Stacks(stacksMap, []string{"network"}, nil)
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

// TestStacks_StackMapWithoutComponentsSectionExcludedByFilter covers the
// defensive guard in stackMatchesAnyComponent: a stack that IS a map but has
// no (or a malformed) "components" section can never satisfy an active
// tags/labels filter.
func TestStacks_StackMapWithoutComponentsSectionExcludedByFilter(t *testing.T) {
	stacksMap := map[string]any{
		"no-components": map[string]any{"vars": map[string]any{"namespace": "acme"}},
	}

	// Without filters the stack is still listed.
	stacks, err := Stacks(stacksMap, nil, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)

	// An active filter can never match a stack whose "components" section is
	// missing.
	stacks, err = Stacks(stacksMap, []string{"network"}, nil)
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

// TestComponentMatchesTagsLabels_NonMapComponentDataNeverMatches covers the
// defensive guard in componentMatchesTagsLabels: a component entry whose
// value is not itself a map (malformed input) cannot satisfy an active
// filter.
func TestComponentMatchesTagsLabels_NonMapComponentDataNeverMatches(t *testing.T) {
	assert.False(t, componentMatchesTagsLabels("not-a-map", []string{"network"}, nil))
	assert.False(t, componentMatchesTagsLabels("not-a-map", nil, map[string]string{"team": "platform"}))
	// Empty filters always match regardless of shape.
	assert.True(t, componentMatchesTagsLabels("not-a-map", nil, nil))
}

func TestStacksForComponent_TagsLabelsFilter(t *testing.T) {
	stacksMap := tagsLabelsStacksMap()

	// vpc matches its own tag in plat-ue2-dev only.
	stacks, err := StacksForComponent("vpc", stacksMap, []string{"network"}, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)
	assert.Equal(t, "plat-ue2-dev", stacks[0]["stack"])
	assert.Equal(t, "vpc", stacks[0]["component"])

	// vpc exists but does not carry the label — empty result, NOT ErrNoStacksFound.
	stacks, err = StacksForComponent("vpc", stacksMap, nil, map[string]string{"team": "platform"})
	require.NoError(t, err)
	assert.Empty(t, stacks)

	// A component that is genuinely absent still errors.
	_, err = StacksForComponent("nonexistent", stacksMap, []string{"network"}, nil)
	assert.ErrorIs(t, err, errUtils.ErrNoStacksFound)
}

func TestStacksForComponent_LabelsAllMatch(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{"team": "platform", "env": "dev"},
						},
					},
				},
			},
		},
	}

	// All requested labels present: match.
	stacks, err := StacksForComponent("vpc", stacksMap, nil, map[string]string{"team": "platform", "env": "dev"})
	require.NoError(t, err)
	assert.Len(t, stacks, 1)

	// One requested label missing: no match (labels are all-match).
	stacks, err = StacksForComponent("vpc", stacksMap, nil, map[string]string{"team": "platform", "region": "us-east-1"})
	require.NoError(t, err)
	assert.Empty(t, stacks)
}

func TestStacks_VarsFromHelmfile(t *testing.T) {
	// Vars should be extracted from any component type.
	stacksMap := map[string]any{
		"plat-ue2-dev": map[string]any{
			"components": map[string]any{
				"helmfile": map[string]any{
					"ingress": map[string]any{
						"vars": map[string]any{
							"namespace": "acme",
							"tenant":    "plat",
						},
					},
				},
			},
		},
	}

	stacks, err := Stacks(stacksMap, nil, nil)
	require.NoError(t, err)
	require.Len(t, stacks, 1)

	stack := stacks[0]
	vars, ok := stack["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", vars["namespace"])
	assert.Equal(t, "plat", vars["tenant"])
}
