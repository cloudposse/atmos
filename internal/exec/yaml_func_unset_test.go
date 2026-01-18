package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestProcessUnsetTag(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name: "unset single key",
			input: map[string]any{
				"key1": "value1",
				"key2": "!unset",
				"key3": "value3",
			},
			expected: map[string]any{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name: "unset nested key",
			input: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child2": "!unset",
					"child3": "value3",
				},
			},
			expected: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child3": "value3",
				},
			},
		},
		{
			name: "unset in array",
			input: map[string]any{
				"items": []any{
					"item1",
					"!unset",
					"item3",
				},
			},
			expected: map[string]any{
				"items": []any{
					"item1",
					"item3",
				},
			},
		},
		{
			name: "unset multiple keys",
			input: map[string]any{
				"key1": "!unset",
				"key2": "value2",
				"key3": "!unset",
				"key4": "value4",
			},
			expected: map[string]any{
				"key2": "value2",
				"key4": "value4",
			},
		},
		{
			name: "unset deeply nested structure",
			input: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"keep":   "value",
							"remove": "!unset",
						},
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"keep": "value",
						},
					},
				},
			},
		},
		{
			name: "unset entire nested map",
			input: map[string]any{
				"keep": "value1",
				"remove": map[string]any{
					"child": "!unset",
				},
				"alsoKeep": "value2",
			},
			expected: map[string]any{
				"keep":     "value1",
				"remove":   map[string]any{},
				"alsoKeep": "value2",
			},
		},
		{
			name: "unset with mixed types",
			input: map[string]any{
				"string": "value",
				"number": 42,
				"bool":   true,
				"remove": "!unset",
				"array":  []any{1, 2, 3},
				"map": map[string]any{
					"nested": "value",
				},
			},
			expected: map[string]any{
				"string": "value",
				"number": 42,
				"bool":   true,
				"array":  []any{1, 2, 3},
				"map": map[string]any{
					"nested": "value",
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processNodes(atmosConfig, tt.input, "", []string{}, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessUnsetWithSkip(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"key1": "value1",
		"key2": "!unset",
		"key3": "value3",
	}

	// Test with unset in skip list - should not process !unset.
	result, err := processNodes(atmosConfig, input, "", []string{"unset"}, nil)
	assert.NoError(t, err)

	expected := map[string]any{
		"key1": "value1",
		"key2": "!unset", // Should remain as is when skipped.
		"key3": "value3",
	}

	assert.Equal(t, expected, result)
}

func TestUnsetMarker(t *testing.T) {
	// Test the UnsetMarker type.
	marker := UnsetMarker{IsUnset: true}
	assert.True(t, marker.IsUnset)

	marker2 := UnsetMarker{IsUnset: false}
	assert.False(t, marker2.IsUnset)
}

func TestProcessCustomTagsWithUnset(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test that processCustomTags returns UnsetMarker for !unset tag.
	result, err := processCustomTags(atmosConfig, "!unset", "", []string{}, nil)
	assert.NoError(t, err)
	marker, ok := result.(UnsetMarker)
	assert.True(t, ok)
	assert.True(t, marker.IsUnset)

	// Test that other tags are still processed normally.
	result2, err2 := processCustomTags(atmosConfig, "!env HOME", "", []string{}, nil)
	assert.NoError(t, err2)
	_, isMarker := result2.(UnsetMarker)
	assert.False(t, isMarker)
}

func TestProcessCustomYamlTagsWithUnset(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := schema.AtmosSectionMapType{
		"vars": map[string]any{
			"enabled": true,
			"remove":  "!unset",
			"config": map[string]any{
				"setting1": "value1",
				"setting2": "!unset",
			},
		},
	}

	expected := schema.AtmosSectionMapType{
		"vars": map[string]any{
			"enabled": true,
			"config": map[string]any{
				"setting1": "value1",
			},
		},
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "", []string{}, nil)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestUnsetInheritanceScenario(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Simulate a parent configuration.
	parent := map[string]any{
		"vars": map[string]any{
			"region":            "us-east-1",
			"instance_type":     "t2.micro",
			"enable_monitoring": true,
		},
	}

	// Simulate a child configuration that wants to unset a parent value.
	child := map[string]any{
		"vars": map[string]any{
			"region":         "us-west-2", // Override.
			"instance_type":  "!unset",    // Remove from configuration.
			"enable_logging": true,        // Add new.
		},
	}

	// Process parent (no unset tags).
	processedParent, err := processNodes(atmosConfig, parent, "", []string{}, nil)
	assert.NoError(t, err)

	// Process child (with unset tag).
	processedChild, err := processNodes(atmosConfig, child, "", []string{}, nil)
	assert.NoError(t, err)

	// Verify the unset tag removed the key from child.
	childVars := processedChild["vars"].(map[string]any)
	_, hasInstanceType := childVars["instance_type"]
	assert.False(t, hasInstanceType, "instance_type should be removed from child config")

	// Verify other keys are preserved.
	assert.Equal(t, "us-west-2", childVars["region"])
	assert.Equal(t, true, childVars["enable_logging"])

	// Parent should remain unchanged.
	parentVars := processedParent["vars"].(map[string]any)
	assert.Equal(t, "t2.micro", parentVars["instance_type"])
}

func TestUnsetWithComplexArray(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"items": []any{
			map[string]any{"id": 1, "name": "item1"},
			"!unset",
			map[string]any{"id": 3, "name": "item3"},
			"!unset",
			map[string]any{"id": 5, "name": "item5"},
		},
	}

	expected := map[string]any{
		"items": []any{
			map[string]any{"id": 1, "name": "item1"},
			map[string]any{"id": 3, "name": "item3"},
			map[string]any{"id": 5, "name": "item5"},
		},
	}

	result, err := processNodes(atmosConfig, input, "", []string{}, nil)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestUnsetEmptyString(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Test that empty string with !unset still works.
	input := map[string]any{
		"key1": "value1",
		"key2": u.AtmosYamlFuncUnset, // Just the tag without any value.
		"key3": "value3",
	}

	expected := map[string]any{
		"key1": "value1",
		"key3": "value3",
	}

	result, err := processNodes(atmosConfig, input, "", []string{}, nil)
	assert.NoError(t, err)
	assert.Equal(t, expected, result)
}
