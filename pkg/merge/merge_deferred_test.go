package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsAtmosYAMLFunction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "detects !template function",
			input:    "!template '{{ .settings.base }}'",
			expected: true,
		},
		{
			name:     "detects !terraform.output function",
			input:    "!terraform.output vpc.id",
			expected: true,
		},
		{
			name:     "detects !terraform.state function",
			input:    "!terraform.state vpc.arn",
			expected: true,
		},
		{
			name:     "detects !store.get function",
			input:    "!store.get secret.key",
			expected: true,
		},
		{
			name:     "detects !store function",
			input:    "!store secret.key",
			expected: true,
		},
		{
			name:     "detects !exec function",
			input:    "!exec echo hello",
			expected: true,
		},
		{
			name:     "detects !env function",
			input:    "!env AWS_REGION",
			expected: true,
		},
		{
			name:     "returns false for regular string",
			input:    "regular string",
			expected: false,
		},
		{
			name:     "returns false for empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "returns false for !include (pre-merge function)",
			input:    "!include catalog/base",
			expected: false,
		},
		{
			name:     "returns false for partial match",
			input:    "template without tag",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAtmosYAMLFunction(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWalkAndDeferYAMLFunctions(t *testing.T) {
	t.Run("defers YAML function strings", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"config": "!template '{{ .settings.base }}'",
			"region": "us-east-1",
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{"vars"})

		// YAML function should be replaced with nil.
		assert.Nil(t, result["config"])
		// Regular values should be preserved.
		assert.Equal(t, "us-east-1", result["region"])

		// Check deferred context.
		assert.True(t, dctx.HasDeferredValues())
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "vars.config")
		assert.Equal(t, "!template '{{ .settings.base }}'", values["vars.config"][0].Value)
	})

	t.Run("recursively processes nested maps", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"template": "!template 'value'",
					"regular":  "string",
				},
			},
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})

		// Navigate to nested value.
		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})

		assert.Nil(t, level2["template"])
		assert.Equal(t, "string", level2["regular"])

		// Check deferred context.
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "level1.level2.template")
	})

	t.Run("preserves non-YAML-function strings", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"normal":  "just a string",
			"number":  42,
			"boolean": true,
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})

		assert.Equal(t, "just a string", result["normal"])
		assert.Equal(t, 42, result["number"])
		assert.Equal(t, true, result["boolean"])
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("handles nil input", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		result := WalkAndDeferYAMLFunctions(dctx, nil, []string{})
		assert.Nil(t, result)
	})

	t.Run("handles empty map", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{}
		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

func TestIsMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{
			name:     "returns true for map[string]interface{}",
			input:    map[string]interface{}{"key": "value"},
			expected: true,
		},
		{
			name:     "returns false for string",
			input:    "string",
			expected: false,
		},
		{
			name:     "returns false for slice",
			input:    []interface{}{1, 2, 3},
			expected: false,
		},
		{
			name:     "returns false for nil",
			input:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{
			name:     "returns true for []interface{}",
			input:    []interface{}{1, 2, 3},
			expected: true,
		},
		{
			name:     "returns false for string",
			input:    "string",
			expected: false,
		},
		{
			name:     "returns false for map",
			input:    map[string]interface{}{"key": "value"},
			expected: false,
		},
		{
			name:     "returns false for nil",
			input:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetValueAtPath(t *testing.T) {
	t.Run("sets value at simple path", func(t *testing.T) {
		data := map[string]interface{}{}
		err := SetValueAtPath(data, []string{"key"}, "value")

		require.NoError(t, err)
		assert.Equal(t, "value", data["key"])
	})

	t.Run("sets value at nested path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{},
			},
		}

		err := SetValueAtPath(data, []string{"level1", "level2", "key"}, "value")

		require.NoError(t, err)
		level1 := data["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		assert.Equal(t, "value", level2["key"])
	})

	t.Run("creates intermediate maps if missing", func(t *testing.T) {
		data := map[string]interface{}{}

		err := SetValueAtPath(data, []string{"new", "nested", "key"}, "value")

		require.NoError(t, err)
		level1 := data["new"].(map[string]interface{})
		level2 := level1["nested"].(map[string]interface{})
		assert.Equal(t, "value", level2["key"])
	})

	t.Run("overwrites existing value", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "old",
		}

		err := SetValueAtPath(data, []string{"key"}, "new")

		require.NoError(t, err)
		assert.Equal(t, "new", data["key"])
	})

	t.Run("returns error for empty path", func(t *testing.T) {
		data := map[string]interface{}{}
		err := SetValueAtPath(data, []string{}, "value")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty path")
	})

	t.Run("returns error when path encounters non-map", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": "string value",
		}

		err := SetValueAtPath(data, []string{"level1", "level2", "key"}, "value")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a map")
	})
}

func TestMergeSlices(t *testing.T) {
	t.Run("replace strategy returns last value", func(t *testing.T) {
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: []interface{}{3, 4}, Precedence: 1},
			{Value: []interface{}{5, 6}, Precedence: 2},
		}

		result, err := mergeSlices(values, ListMergeStrategyReplace)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{5, 6}, result)
	})

	t.Run("append strategy concatenates all slices", func(t *testing.T) {
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: []interface{}{3, 4}, Precedence: 1},
			{Value: []interface{}{5, 6}, Precedence: 2},
		}

		result, err := mergeSlices(values, ListMergeStrategyAppend)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{1, 2, 3, 4, 5, 6}, result)
	})

	t.Run("merge strategy deep-merges by index", func(t *testing.T) {
		values := []*DeferredValue{
			{
				Value: []interface{}{
					map[string]interface{}{"a": 1, "b": 2},
					map[string]interface{}{"c": 3},
				},
				Precedence: 0,
			},
			{
				Value: []interface{}{
					map[string]interface{}{"b": 20, "d": 4},
				},
				Precedence: 1,
			},
		}

		result, err := mergeSlices(values, ListMergeStrategyMerge)

		require.NoError(t, err)
		resultSlice := result.([]interface{})
		assert.Len(t, resultSlice, 2)

		// First item should be deep-merged.
		firstItem := resultSlice[0].(map[string]interface{})
		assert.Equal(t, 1, firstItem["a"])
		assert.Equal(t, 20, firstItem["b"]) // Overridden.
		assert.Equal(t, 4, firstItem["d"])

		// Second item from first slice.
		secondItem := resultSlice[1].(map[string]interface{})
		assert.Equal(t, 3, secondItem["c"])
	})

	t.Run("merge strategy with non-map items replaces by index", func(t *testing.T) {
		values := []*DeferredValue{
			{Value: []interface{}{"a", "b", "c"}, Precedence: 0},
			{Value: []interface{}{"x", "y"}, Precedence: 1},
		}

		result, err := mergeSlices(values, ListMergeStrategyMerge)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{"x", "y", "c"}, result)
	})

	t.Run("handles empty values slice", func(t *testing.T) {
		values := []*DeferredValue{}

		result, err := mergeSlices(values, ListMergeStrategyReplace)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("skips non-slice values in append", func(t *testing.T) {
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: "not a slice", Precedence: 1},
			{Value: []interface{}{3, 4}, Precedence: 2},
		}

		result, err := mergeSlices(values, ListMergeStrategyAppend)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, result)
	})

	t.Run("returns error for unknown strategy", func(t *testing.T) {
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
		}

		result, err := mergeSlices(values, "unknown")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown list merge strategy")
		assert.Nil(t, result)
	})
}

func TestMergeDeferredValues(t *testing.T) {
	t.Run("returns nil for empty values", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{}
		result, err := MergeDeferredValues([]*DeferredValue{}, &cfg)

		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("returns single value unchanged", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{}
		values := []*DeferredValue{
			{Value: "test", Precedence: 0},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		assert.Equal(t, "test", result)
	})

	t.Run("merges maps using deep merge", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{}
		values := []*DeferredValue{
			{
				Value:      map[string]interface{}{"a": 1, "b": 2},
				Precedence: 0,
			},
			{
				Value:      map[string]interface{}{"b": 20, "c": 3},
				Precedence: 1,
			},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		resultMap := result.(map[string]interface{})
		assert.Equal(t, 1, resultMap["a"])
		assert.Equal(t, 20, resultMap["b"]) // Overridden.
		assert.Equal(t, 3, resultMap["c"])
	})

	t.Run("merges slices with replace strategy", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: []interface{}{3, 4}, Precedence: 1},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{3, 4}, result)
	})

	t.Run("merges slices with append strategy", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyAppend,
			},
		}
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: []interface{}{3, 4}, Precedence: 1},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{1, 2, 3, 4}, result)
	})

	t.Run("uses default replace strategy when not specified", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{}
		values := []*DeferredValue{
			{Value: []interface{}{1, 2}, Precedence: 0},
			{Value: []interface{}{3, 4}, Precedence: 1},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		assert.Equal(t, []interface{}{3, 4}, result)
	})

	t.Run("last simple value wins", func(t *testing.T) {
		cfg := schema.AtmosConfiguration{}
		values := []*DeferredValue{
			{Value: "first", Precedence: 0},
			{Value: "second", Precedence: 1},
			{Value: "third", Precedence: 2},
		}

		result, err := MergeDeferredValues(values, &cfg)

		require.NoError(t, err)
		assert.Equal(t, "third", result)
	})
}

func TestMergeWithDeferred(t *testing.T) {
	t.Run("merges inputs and returns deferred context", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{
				"template": "!template 'value1'",
				"regular":  "string1",
			},
			{
				"template": "!template 'value2'",
				"regular":  "string2",
			},
		}

		result, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, dctx)

		// YAML function should be replaced with nil.
		assert.Nil(t, result["template"])
		// Regular value should be merged (last wins).
		assert.Equal(t, "string2", result["regular"])

		// Deferred context should have the YAML functions.
		assert.True(t, dctx.HasDeferredValues())
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "template")
		assert.Len(t, values["template"], 2)
	})

	t.Run("increments precedence for each input", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{"func": "!template 'first'"},
			{"func": "!template 'second'"},
			{"func": "!template 'third'"},
		}

		_, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		values := dctx.GetDeferredValues()["func"]
		assert.Len(t, values, 3)
		assert.Equal(t, 0, values[0].Precedence)
		assert.Equal(t, 1, values[1].Precedence)
		assert.Equal(t, 2, values[2].Precedence)
	})

	t.Run("handles inputs without YAML functions", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		inputs := []map[string]any{
			{"key1": "value1"},
			{"key2": "value2"},
		}

		result, dctx, err := MergeWithDeferred(cfg, inputs)

		require.NoError(t, err)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
		assert.False(t, dctx.HasDeferredValues())
	})
}

func TestApplyDeferredMerges(t *testing.T) {
	t.Run("returns nil error when context is nil", func(t *testing.T) {
		result := map[string]interface{}{}
		err := ApplyDeferredMerges(nil, result, nil)
		assert.NoError(t, err)
	})

	t.Run("returns nil error when no deferred values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		result := map[string]interface{}{}
		err := ApplyDeferredMerges(dctx, result, nil)
		assert.NoError(t, err)
	})

	t.Run("applies deferred values to result", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		// Simulate deferred YAML function strings.
		// Note: These won't be processed (TODO in ApplyDeferredMerges),
		// but will be merged as strings.
		dctx.AddDeferred([]string{"config"}, "!template 'value'")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg)

		require.NoError(t, err)
		// The value should be set (as the string, since processing is TODO).
		assert.Equal(t, "!template 'value'", result["config"])
	})

	t.Run("sorts by precedence before merging", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		// Add in non-sorted order.
		dctx.precedence = 2
		dctx.AddDeferred([]string{"key"}, "third")
		dctx.precedence = 0
		dctx.AddDeferred([]string{"key"}, "first")
		dctx.precedence = 1
		dctx.AddDeferred([]string{"key"}, "second")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg)

		require.NoError(t, err)
		// With replace strategy, last (highest precedence) should win.
		assert.Equal(t, "third", result["key"])
	})

	t.Run("handles nested paths", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: ListMergeStrategyReplace,
			},
		}

		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"level1", "level2", "key"}, "value")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg)

		require.NoError(t, err)

		level1 := result["level1"].(map[string]interface{})
		level2 := level1["level2"].(map[string]interface{})
		assert.Equal(t, "value", level2["key"])
	})

	t.Run("uses default strategy when atmosConfig is nil", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, []interface{}{1, 2})
		dctx.IncrementPrecedence()
		dctx.AddDeferred([]string{"key"}, []interface{}{3, 4})

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, nil)

		require.NoError(t, err)
		// Default is replace strategy, so last value wins.
		assert.Equal(t, []interface{}{3, 4}, result["key"])
	})
}
