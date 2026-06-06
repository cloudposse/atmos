package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMergeWithAppendTag(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	tests := []struct {
		name     string
		inputs   []map[string]any
		expected map[string]any
	}{
		{
			name: "append tag on depends_on list",
			inputs: []map[string]any{
				{
					"components": map[string]any{
						"terraform": map[string]any{
							"eks": map[string]any{
								"settings": map[string]any{
									"depends_on": []any{"vpc", "iam-role"},
								},
							},
						},
					},
				},
				{
					"components": map[string]any{
						"terraform": map[string]any{
							"eks": map[string]any{
								"settings": map[string]any{
									"depends_on": u.WrapWithAppendTag([]any{"rds", "elasticache"}),
								},
							},
						},
					},
				},
			},
			expected: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks": map[string]any{
							"settings": map[string]any{
								"depends_on": []any{"vpc", "iam-role", "rds", "elasticache"},
							},
						},
					},
				},
			},
		},
		{
			name: "append tag with no existing list",
			inputs: []map[string]any{
				{
					"components": map[string]any{
						"terraform": map[string]any{
							"eks": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
							},
						},
					},
				},
				{
					"components": map[string]any{
						"terraform": map[string]any{
							"eks": map[string]any{
								"settings": map[string]any{
									"depends_on": u.WrapWithAppendTag([]any{"vpc", "iam-role"}),
								},
							},
						},
					},
				},
			},
			expected: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"eks": map[string]any{
							"vars": map[string]any{
								"region": "us-east-1",
							},
							"settings": map[string]any{
								"depends_on": []any{"vpc", "iam-role"},
							},
						},
					},
				},
			},
		},
		{
			name: "regular list replacement without append tag",
			inputs: []map[string]any{
				{
					"list": []any{"item1", "item2"},
				},
				{
					"list": []any{"item3", "item4"},
				},
			},
			expected: map[string]any{
				"list": []any{"item3", "item4"},
			},
		},
		{
			name: "mixed append and regular lists",
			inputs: []map[string]any{
				{
					"append_list":  []any{"a", "b"},
					"regular_list": []any{"1", "2"},
				},
				{
					"append_list":  u.WrapWithAppendTag([]any{"c", "d"}),
					"regular_list": []any{"3", "4"},
				},
			},
			expected: map[string]any{
				"append_list":  []any{"a", "b", "c", "d"},
				"regular_list": []any{"3", "4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Merge(&atmosConfig, tt.inputs)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMergeWithAppendTagGlobalStrategy(t *testing.T) {
	// Test that append tag works alongside global append strategy
	// When global append is on, both strategies append (no conflict)
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyAppend, // Global append
		},
	}

	inputs := []map[string]any{
		{
			"global_append_list": []any{"a", "b"},
			"tagged_append_list": []any{"1", "2"},
		},
		{
			"global_append_list": []any{"c", "d"},
			"tagged_append_list": u.WrapWithAppendTag([]any{"3", "4"}),
		},
	}

	expected := map[string]any{
		"global_append_list": []any{"a", "b", "c", "d"}, // Uses global append strategy.
		// With the global append strategy on, processAppendTags returns only the new !append
		// items so the append happens exactly once (no duplication of the base list).
		"tagged_append_list": []any{"1", "2", "3", "4"},
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

// TestMergeAppend_ChainedAcrossThreeInputs verifies that !append accumulates across more
// than two inputs (e.g. base + two successive overlay imports), each appending to the
// result of the previous merge.
func TestMergeAppend_ChainedAcrossThreeInputs(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	inputs := []map[string]any{
		{"items": []any{"a"}},
		{"items": u.WrapWithAppendTag([]any{"b", "c"})},
		{"items": u.WrapWithAppendTag([]any{"d"})},
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"items": []any{"a", "b", "c", "d"}}, result)
}

// TestMergeAppend_SingleInputResolvesWrapper verifies that a lone !append (the single-input
// fast path) resolves to a plain list rather than leaking the __atmos_append__ wrapper.
func TestMergeAppend_SingleInputResolvesWrapper(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	inputs := []map[string]any{
		{"items": u.WrapWithAppendTag([]any{"a", "b"})},
	}

	result, err := Merge(&atmosConfig, inputs)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"items": []any{"a", "b"}}, result,
		"a single-input !append must resolve to a plain list, not leak the wrapper")
}

// TestMergeAppend_BaseInputAppendTag verifies that an !append tag in the first (base)
// input has nothing to append to and therefore resolves to a plain list, which a later
// override then appends to.
func TestMergeAppend_BaseInputAppendTag(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	inputs := []map[string]any{
		{"items": u.WrapWithAppendTag([]any{"a", "b"})},
		{"items": u.WrapWithAppendTag([]any{"c"})},
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"items": []any{"a", "b", "c"}}, result)
}

// TestMergeAppend_ListOfMaps verifies that !append works for lists whose elements are
// maps (e.g. node_groups), not just scalars, preserving each element.
func TestMergeAppend_ListOfMaps(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	base := map[string]any{
		"node_groups": []any{
			map[string]any{"name": "default", "instance_type": "t3.medium"},
		},
	}
	overlay := map[string]any{
		"node_groups": u.WrapWithAppendTag([]any{
			map[string]any{"name": "spot", "instance_type": "t3.large"},
		}),
	}

	result, err := Merge(&atmosConfig, []map[string]any{base, overlay})
	assert.NoError(t, err)

	got, ok := result["node_groups"].([]any)
	assert.True(t, ok, "node_groups should be a slice")
	assert.Len(t, got, 2)
	assert.Equal(t, map[string]any{"name": "default", "instance_type": "t3.medium"}, got[0])
	assert.Equal(t, map[string]any{"name": "spot", "instance_type": "t3.large"}, got[1])
}

// TestMergeAppend_ExistingValueNotAList verifies that when the accumulator's value for the
// key is not a list (type mismatch), !append falls back to just the new items rather than
// erroring or panicking.
func TestMergeAppend_ExistingValueNotAList(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	inputs := []map[string]any{
		{"thing": "i-am-a-scalar"},
		{"thing": u.WrapWithAppendTag([]any{"x", "y"})},
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"thing": []any{"x", "y"}}, result)
}

// TestMergeAppend_NewKey verifies that an !append on a key absent from the base resolves
// to just the appended list.
func TestMergeAppend_NewKey(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	inputs := []map[string]any{
		{"existing": "value"},
		{"fresh": u.WrapWithAppendTag([]any{"a", "b"})},
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, map[string]any{"existing": "value", "fresh": []any{"a", "b"}}, result)
}

// TestMergeAppend_DoesNotMutateInputs verifies that resolving !append tags during merge
// does not mutate the caller's input maps (immutability contract).
func TestMergeAppend_DoesNotMutateInputs(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	base := map[string]any{"items": []any{"a"}}
	overlayList := []any{"b"}
	overlay := map[string]any{"items": u.WrapWithAppendTag(overlayList)}

	result, err := Merge(&atmosConfig, []map[string]any{base, overlay})
	assert.NoError(t, err)
	assert.Equal(t, []any{"a", "b"}, result["items"])

	// result -> src isolation: mutating the merged result must not affect the inputs.
	result["items"].([]any)[0] = "MUTATED"
	assert.Equal(t, []any{"a"}, base["items"], "base input must not be mutated by the merge")
	assert.Equal(t, []any{"b"}, overlayList, "overlay input list must not be mutated by the merge")

	// src -> result isolation: mutating a source input after the merge must not affect the result.
	overlayList[0] = "SOURCE_MUTATED"
	assert.Equal(t, []any{"MUTATED", "b"}, result["items"], "merged result must not alias source slices")
}

// TestMergeAppend_EndToEndFromStackManifests is the full-path test: it unmarshals two stack
// manifests (a base and an overlay using !append) exactly as the stack processor does, then
// merges them. This proves !append is wired through stack-manifest parsing (the !append tag
// is rewritten into the append wrapper during unmarshal) and appends rather than replaces.
func TestMergeAppend_EndToEndFromStackManifests(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{ListMergeStrategy: ListMergeStrategyReplace},
	}

	baseYAML := `components:
  terraform:
    eks:
      vars:
        cluster_name: main
        node_groups:
          - name: default
            instance_type: t3.medium
      settings:
        depends_on:
          - vpc
          - iam-role
`
	overlayYAML := `components:
  terraform:
    eks:
      vars:
        cluster_name: production
        node_groups: !append
          - name: spot
            instance_type: t3.large
      settings:
        depends_on: !append
          - rds
          - elasticache
`
	base, err := u.UnmarshalYAMLFromFile[map[string]any](cfg, baseYAML, "base.yaml")
	require.NoError(t, err)
	overlay, err := u.UnmarshalYAMLFromFile[map[string]any](cfg, overlayYAML, "overlay.yaml")
	require.NoError(t, err)

	result, err := Merge(cfg, []map[string]any{base, overlay})
	require.NoError(t, err)

	eks := result["components"].(map[string]any)["terraform"].(map[string]any)["eks"].(map[string]any)
	vars := eks["vars"].(map[string]any)
	settings := eks["settings"].(map[string]any)

	// Scalar sibling is overridden as usual.
	assert.Equal(t, "production", vars["cluster_name"])

	// node_groups (list of maps) is appended, not replaced.
	nodeGroups, ok := vars["node_groups"].([]any)
	require.True(t, ok, "node_groups should be a plain slice after merge, got %#v", vars["node_groups"])
	require.Len(t, nodeGroups, 2)
	assert.Equal(t, "default", nodeGroups[0].(map[string]any)["name"])
	assert.Equal(t, "spot", nodeGroups[1].(map[string]any)["name"])

	// depends_on (list of strings) is appended, not replaced.
	assert.Equal(t, []any{"vpc", "iam-role", "rds", "elasticache"}, settings["depends_on"])
}
