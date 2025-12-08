package merge

import (
	"errors"
	"fmt"
	"strings"
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

		// Navigate to nested value using require guards to prevent panics on type mismatch.
		level1, ok := result["level1"].(map[string]interface{})
		require.True(t, ok, "level1 should be a map")
		level2, ok := level1["level2"].(map[string]interface{})
		require.True(t, ok, "level2 should be a map")

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
		err := ApplyDeferredMerges(nil, result, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("returns nil error when no deferred values", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		result := map[string]interface{}{}
		err := ApplyDeferredMerges(dctx, result, nil, nil)
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
		// Note: These won't be processed (processor is nil),
		// but will be merged as strings.
		dctx.AddDeferred([]string{"config"}, "!template 'value'")

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

		require.NoError(t, err)
		// The value should be set (as the string, since no processor was provided).
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

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

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

		err := ApplyDeferredMerges(dctx, result, cfg, nil)

		require.NoError(t, err)

		// Use require guards for type assertions to provide clear test failures instead of panics.
		level1, ok := result["level1"].(map[string]interface{})
		require.True(t, ok, "level1 should be a map")
		level2, ok := level1["level2"].(map[string]interface{})
		require.True(t, ok, "level2 should be a map")

		assert.Equal(t, "value", level2["key"])
	})

	t.Run("uses default strategy when atmosConfig is nil", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		dctx.AddDeferred([]string{"key"}, []interface{}{1, 2})
		dctx.IncrementPrecedence()
		dctx.AddDeferred([]string{"key"}, []interface{}{3, 4})

		result := map[string]interface{}{}

		err := ApplyDeferredMerges(dctx, result, nil, nil)

		require.NoError(t, err)
		// Default is replace strategy, so last value wins.
		assert.Equal(t, []interface{}{3, 4}, result["key"])
	})
}

// TestProcessYAMLFunctions tests the processYAMLFunctions helper function.
func TestProcessYAMLFunctions(t *testing.T) {
	t.Run("processes YAML functions successfully", func(t *testing.T) {
		// Create a mock processor.
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				// Simple processor that converts "!template X" to uppercase.
				if strings.HasPrefix(value, "!template ") {
					return strings.ToUpper(strings.TrimPrefix(value, "!template ")), nil
				}
				return value, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template hello", IsFunction: true},
			{Value: "!template world", IsFunction: true},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, "HELLO", deferredValues[0].Value)
		assert.False(t, deferredValues[0].IsFunction)
		assert.Equal(t, "WORLD", deferredValues[1].Value)
		assert.False(t, deferredValues[1].IsFunction)
	})

	t.Run("skips non-function values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for non-function values")
				return nil, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "regular string", IsFunction: false},
			{Value: 123, IsFunction: false},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, "regular string", deferredValues[0].Value)
		assert.Equal(t, 123, deferredValues[1].Value)
	})

	t.Run("skips non-string function values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for non-string values")
				return nil, nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: 123, IsFunction: true},           // Non-string but marked as function.
			{Value: []string{"a"}, IsFunction: true}, // Non-string but marked as function.
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		// Values should remain unchanged.
		assert.Equal(t, 123, deferredValues[0].Value)
		assert.Equal(t, []string{"a"}, deferredValues[1].Value)
	})

	t.Run("returns error on processing failure", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				return nil, errors.New("processing failed")
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template error", IsFunction: true},
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to process YAML function at test.path")
		assert.Contains(t, err.Error(), "processing failed")
	})

	t.Run("processes mixed values correctly", func(t *testing.T) {
		processCount := 0
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				processCount++
				return "processed", nil
			},
		}

		deferredValues := []*DeferredValue{
			{Value: "!template func1", IsFunction: true}, // Should process.
			{Value: "regular", IsFunction: false},        // Should skip.
			{Value: "!template func2", IsFunction: true}, // Should process.
			{Value: 123, IsFunction: true},               // Should skip (non-string).
		}

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
		assert.Equal(t, 2, processCount, "should process exactly 2 values")
		assert.Equal(t, "processed", deferredValues[0].Value)
		assert.False(t, deferredValues[0].IsFunction)
		assert.Equal(t, "regular", deferredValues[1].Value)
		assert.Equal(t, "processed", deferredValues[2].Value)
		assert.False(t, deferredValues[2].IsFunction)
		assert.Equal(t, 123, deferredValues[3].Value)
	})

	t.Run("handles empty deferred values", func(t *testing.T) {
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				t.Fatal("processor should not be called for empty slice")
				return nil, nil
			},
		}

		var deferredValues []*DeferredValue

		err := processYAMLFunctions(deferredValues, processor, "test.path")

		require.NoError(t, err)
	})
}

// TestGetValueAtPath tests the GetValueAtPath function.
func TestGetValueAtPath(t *testing.T) {
	t.Run("gets value at top-level path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Equal(t, "value", value)
	})

	t.Run("gets value at nested path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"key": "nested_value",
				},
			},
		}
		path := []string{"level1", "level2", "key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Equal(t, "nested_value", value)
	})

	t.Run("returns false for non-existent path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{"nonexistent"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for partial path", func(t *testing.T) {
		data := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": "value",
			},
		}
		path := []string{"level1", "level2", "level3"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for nil data", func(t *testing.T) {
		var data map[string]interface{}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for empty path", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}
		path := []string{}

		value, exists := GetValueAtPath(data, path)

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("handles nil values", func(t *testing.T) {
		data := map[string]interface{}{
			"key": nil,
		}
		path := []string{"key"}

		value, exists := GetValueAtPath(data, path)

		assert.True(t, exists)
		assert.Nil(t, value)
	})
}

// TestGetConfigOrDefault tests the getConfigOrDefault function.
func TestGetConfigOrDefault(t *testing.T) {
	t.Run("returns provided config when not nil", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "append",
			},
		}

		result := getConfigOrDefault(cfg)

		assert.Equal(t, cfg, result)
		assert.Equal(t, "append", result.Settings.ListMergeStrategy)
	})

	t.Run("returns default config when nil", func(t *testing.T) {
		result := getConfigOrDefault(nil)

		assert.NotNil(t, result)
		assert.Equal(t, "", result.Settings.ListMergeStrategy)
	})
}

// TestFindMaxPrecedence tests the findMaxPrecedence function.
func TestFindMaxPrecedence(t *testing.T) {
	t.Run("returns max precedence from multiple values", func(t *testing.T) {
		values := []*DeferredValue{
			{Precedence: 0},
			{Precedence: 5},
			{Precedence: 2},
			{Precedence: 8},
			{Precedence: 3},
		}

		max := findMaxPrecedence(values)

		assert.Equal(t, 8, max)
	})

	t.Run("returns first precedence when only one value", func(t *testing.T) {
		values := []*DeferredValue{
			{Precedence: 42},
		}

		max := findMaxPrecedence(values)

		assert.Equal(t, 42, max)
	})

	t.Run("returns zero for empty slice", func(t *testing.T) {
		values := []*DeferredValue{}

		max := findMaxPrecedence(values)

		assert.Equal(t, 0, max)
	})

	t.Run("handles all same precedence", func(t *testing.T) {
		values := []*DeferredValue{
			{Precedence: 5},
			{Precedence: 5},
			{Precedence: 5},
		}

		max := findMaxPrecedence(values)

		assert.Equal(t, 5, max)
	})
}

// TestAddExistingConcreteValue tests the addExistingConcreteValue function.
func TestAddExistingConcreteValue(t *testing.T) {
	t.Run("adds existing non-nil value with highest precedence", func(t *testing.T) {
		result := map[string]interface{}{
			"key": "existing_value",
		}
		deferredValues := []*DeferredValue{
			{Path: []string{"key"}, Value: "value1", Precedence: 0},
			{Path: []string{"key"}, Value: "value2", Precedence: 1},
		}

		updated := addExistingConcreteValue(result, deferredValues)

		assert.Len(t, updated, 3)
		assert.Equal(t, "existing_value", updated[2].Value)
		assert.Equal(t, 2, updated[2].Precedence) // maxPrecedence + 1
		assert.False(t, updated[2].IsFunction)
	})

	t.Run("returns unchanged when no existing value", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{Path: []string{"key"}, Value: "value1", Precedence: 0},
		}

		updated := addExistingConcreteValue(result, deferredValues)

		assert.Len(t, updated, 1)
		assert.Equal(t, deferredValues, updated)
	})

	t.Run("returns unchanged when existing value is nil", func(t *testing.T) {
		result := map[string]interface{}{
			"key": nil,
		}
		deferredValues := []*DeferredValue{
			{Path: []string{"key"}, Value: "value1", Precedence: 0},
		}

		updated := addExistingConcreteValue(result, deferredValues)

		assert.Len(t, updated, 1)
		assert.Equal(t, deferredValues, updated)
	})

	t.Run("handles nested paths", func(t *testing.T) {
		result := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": "nested_value",
			},
		}
		deferredValues := []*DeferredValue{
			{Path: []string{"level1", "level2"}, Value: "value1", Precedence: 0},
			{Path: []string{"level1", "level2"}, Value: "value2", Precedence: 3},
		}

		updated := addExistingConcreteValue(result, deferredValues)

		assert.Len(t, updated, 3)
		assert.Equal(t, "nested_value", updated[2].Value)
		assert.Equal(t, 4, updated[2].Precedence) // maxPrecedence (3) + 1
	})
}

// TestProcessDeferredField tests the processDeferredField function.
func TestProcessDeferredField(t *testing.T) {
	t.Run("processes field with yaml functions", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'value1'",
				Precedence: 0,
				IsFunction: true,
			},
			{
				Path:       []string{"config"},
				Value:      "!template 'value2'",
				Precedence: 1,
				IsFunction: true,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				// Simulate processing templates.
				if value == "!template 'value1'" {
					return "processed1", nil
				}
				return "processed2", nil
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, processor)

		assert.NoError(t, err)
		assert.Equal(t, "processed2", result["config"]) // Higher precedence wins.
	})

	t.Run("processes field without yaml functions", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "value1",
				Precedence: 0,
				IsFunction: false,
			},
			{
				Path:       []string{"config"},
				Value:      "value2",
				Precedence: 1,
				IsFunction: false,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, nil)

		assert.NoError(t, err)
		assert.Equal(t, "value2", result["config"]) // Higher precedence wins.
	})

	t.Run("includes existing concrete value", func(t *testing.T) {
		result := map[string]interface{}{
			"config": "existing",
		}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'deferred'",
				Precedence: 0,
				IsFunction: true,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, nil)

		assert.NoError(t, err)
		// Existing concrete value should win (highest precedence).
		assert.Equal(t, "existing", result["config"])
	})

	t.Run("handles processor error", func(t *testing.T) {
		result := map[string]interface{}{}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"config"},
				Value:      "!template 'invalid'",
				Precedence: 0,
				IsFunction: true,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}
		processor := &mockYAMLProcessor{
			processFunc: func(value string) (any, error) {
				return nil, fmt.Errorf("template processing failed")
			},
		}

		err := processDeferredField("config", deferredValues, result, cfg, processor)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "template processing failed")
	})

	t.Run("handles path navigation error", func(t *testing.T) {
		// Create a result where the path cannot be set (non-map intermediate value).
		result := map[string]interface{}{
			"level1": "string_value", // This is not a map, so we can't navigate deeper.
		}
		deferredValues := []*DeferredValue{
			{
				Path:       []string{"level1", "level2", "key"},
				Value:      "value",
				Precedence: 0,
				IsFunction: false,
			},
		}
		cfg := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				ListMergeStrategy: "replace",
			},
		}

		err := processDeferredField("level1.level2.key", deferredValues, result, cfg, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set value at level1.level2.key")
	})
}

// mockYAMLProcessor is a mock implementation of YAMLFunctionProcessor for testing.
type mockYAMLProcessor struct {
	processFunc func(value string) (any, error)
}

func (m *mockYAMLProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	if m.processFunc != nil {
		return m.processFunc(value)
	}
	return value, nil
}
