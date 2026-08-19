package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

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
