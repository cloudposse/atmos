package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
		"global_append_list": []any{"a", "b", "c", "d"},           // Uses global append strategy
		"tagged_append_list": []any{"1", "2", "1", "2", "3", "4"}, // Both global and tag append apply
	}

	result, err := Merge(&atmosConfig, inputs)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}
