package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// TestGoToCty tests the GoToCty function with various input types.
func TestGoToCty(t *testing.T) {
	t.Run("converts nil to cty.NilVal", func(t *testing.T) {
		result := GoToCty(nil)
		assert.True(t, result.IsNull(), "nil should convert to cty.NilVal")
	})

	t.Run("converts string to cty.StringVal", func(t *testing.T) {
		result := GoToCty("hello world")
		require.True(t, result.Type() == cty.String, "should be string type")
		assert.Equal(t, "hello world", result.AsString())
	})

	t.Run("converts empty string to cty.StringVal", func(t *testing.T) {
		result := GoToCty("")
		require.True(t, result.Type() == cty.String, "should be string type")
		assert.Equal(t, "", result.AsString())
	})

	t.Run("converts bool true to cty.BoolVal", func(t *testing.T) {
		result := GoToCty(true)
		require.True(t, result.Type() == cty.Bool, "should be bool type")
		assert.True(t, result.True())
	})

	t.Run("converts bool false to cty.BoolVal", func(t *testing.T) {
		result := GoToCty(false)
		require.True(t, result.Type() == cty.Bool, "should be bool type")
		assert.False(t, result.True())
	})

	t.Run("converts int to cty.NumberIntVal", func(t *testing.T) {
		result := GoToCty(42)
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		intVal, _ := bigFloat.Int64()
		assert.Equal(t, int64(42), intVal)
	})

	t.Run("converts negative int to cty.NumberIntVal", func(t *testing.T) {
		result := GoToCty(-42)
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		intVal, _ := bigFloat.Int64()
		assert.Equal(t, int64(-42), intVal)
	})

	t.Run("converts zero int to cty.NumberIntVal", func(t *testing.T) {
		result := GoToCty(0)
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		intVal, _ := bigFloat.Int64()
		assert.Equal(t, int64(0), intVal)
	})

	t.Run("converts int64 to cty.NumberIntVal", func(t *testing.T) {
		result := GoToCty(int64(123456789012345))
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		intVal, _ := bigFloat.Int64()
		assert.Equal(t, int64(123456789012345), intVal)
	})

	t.Run("converts uint64 to cty.NumberUIntVal", func(t *testing.T) {
		result := GoToCty(uint64(999))
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		uintVal, _ := bigFloat.Uint64()
		assert.Equal(t, uint64(999), uintVal)
	})

	t.Run("converts float64 to cty.NumberFloatVal", func(t *testing.T) {
		result := GoToCty(3.14159)
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		floatVal, _ := bigFloat.Float64()
		assert.InDelta(t, 3.14159, floatVal, 0.00001)
	})

	t.Run("converts negative float64 to cty.NumberFloatVal", func(t *testing.T) {
		result := GoToCty(-2.71828)
		require.True(t, result.Type() == cty.Number, "should be number type")
		bigFloat := result.AsBigFloat()
		floatVal, _ := bigFloat.Float64()
		assert.InDelta(t, -2.71828, floatVal, 0.00001)
	})

	t.Run("converts simple map to cty.ObjectVal", func(t *testing.T) {
		input := map[string]any{
			"name":    "test",
			"count":   42,
			"enabled": true,
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		assert.Equal(t, "test", valueMap["name"].AsString())

		countBigFloat := valueMap["count"].AsBigFloat()
		countInt, _ := countBigFloat.Int64()
		assert.Equal(t, int64(42), countInt)

		assert.True(t, valueMap["enabled"].True())
	})

	t.Run("converts empty map to cty.ObjectVal", func(t *testing.T) {
		input := map[string]any{}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		assert.Empty(t, valueMap, "empty map should produce empty object")
	})

	t.Run("converts nested map to cty.ObjectVal recursively", func(t *testing.T) {
		input := map[string]any{
			"outer": map[string]any{
				"inner": map[string]any{
					"value": "nested",
				},
			},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		outerMap := valueMap["outer"].AsValueMap()
		innerMap := outerMap["inner"].AsValueMap()
		assert.Equal(t, "nested", innerMap["value"].AsString())
	})

	t.Run("converts map with mixed types to cty.ObjectVal", func(t *testing.T) {
		input := map[string]any{
			"string": "hello",
			"int":    42,
			"bool":   true,
			"float":  3.14,
			"nested": map[string]any{
				"key": "value",
			},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		assert.Equal(t, "hello", valueMap["string"].AsString())

		intBigFloat := valueMap["int"].AsBigFloat()
		intVal, _ := intBigFloat.Int64()
		assert.Equal(t, int64(42), intVal)

		assert.True(t, valueMap["bool"].True())

		floatBigFloat := valueMap["float"].AsBigFloat()
		floatVal, _ := floatBigFloat.Float64()
		assert.InDelta(t, 3.14, floatVal, 0.001)

		nestedMap := valueMap["nested"].AsValueMap()
		assert.Equal(t, "value", nestedMap["key"].AsString())
	})

	t.Run("converts simple slice to cty.TupleVal", func(t *testing.T) {
		input := []any{"one", "two", "three"}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 3)
		assert.Equal(t, "one", valueSlice[0].AsString())
		assert.Equal(t, "two", valueSlice[1].AsString())
		assert.Equal(t, "three", valueSlice[2].AsString())
	})

	t.Run("converts empty slice to cty.EmptyTupleVal", func(t *testing.T) {
		input := []any{}

		result := GoToCty(input)
		assert.True(t, result.Equals(cty.EmptyTupleVal).True(), "empty slice should convert to EmptyTupleVal")
	})

	t.Run("converts slice with mixed types to cty.TupleVal", func(t *testing.T) {
		input := []any{"string", 42, true, 3.14}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 4)
		assert.Equal(t, "string", valueSlice[0].AsString())

		intBigFloat := valueSlice[1].AsBigFloat()
		intVal, _ := intBigFloat.Int64()
		assert.Equal(t, int64(42), intVal)

		assert.True(t, valueSlice[2].True())

		floatBigFloat := valueSlice[3].AsBigFloat()
		floatVal, _ := floatBigFloat.Float64()
		assert.InDelta(t, 3.14, floatVal, 0.001)
	})

	t.Run("converts nested slice to cty.TupleVal recursively", func(t *testing.T) {
		input := []any{
			[]any{"inner1", "inner2"},
			[]any{"inner3", "inner4"},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 2)

		inner1 := valueSlice[0].AsValueSlice()
		assert.Equal(t, "inner1", inner1[0].AsString())
		assert.Equal(t, "inner2", inner1[1].AsString())

		inner2 := valueSlice[1].AsValueSlice()
		assert.Equal(t, "inner3", inner2[0].AsString())
		assert.Equal(t, "inner4", inner2[1].AsString())
	})

	t.Run("converts slice containing maps to cty.TupleVal", func(t *testing.T) {
		input := []any{
			map[string]any{"key1": "value1"},
			map[string]any{"key2": "value2"},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 2)

		map1 := valueSlice[0].AsValueMap()
		assert.Equal(t, "value1", map1["key1"].AsString())

		map2 := valueSlice[1].AsValueMap()
		assert.Equal(t, "value2", map2["key2"].AsString())
	})

	t.Run("converts map containing slices to cty.ObjectVal", func(t *testing.T) {
		input := map[string]any{
			"items": []any{"item1", "item2", "item3"},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		itemsSlice := valueMap["items"].AsValueSlice()
		assert.Len(t, itemsSlice, 3)
		assert.Equal(t, "item1", itemsSlice[0].AsString())
		assert.Equal(t, "item2", itemsSlice[1].AsString())
		assert.Equal(t, "item3", itemsSlice[2].AsString())
	})

	t.Run("converts complex nested structure", func(t *testing.T) {
		input := map[string]any{
			"assume_role": map[string]any{
				"role_arn":     "arn:aws:iam::123456:role/test",
				"session_name": "terraform",
				"duration":     "1h",
				"tags": map[string]any{
					"Environment": "prod",
					"Team":        "platform",
				},
			},
			"allowed_account_ids": []any{"123456", "234567"},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()

		// Check assume_role
		assumeRole := valueMap["assume_role"].AsValueMap()
		assert.Equal(t, "arn:aws:iam::123456:role/test", assumeRole["role_arn"].AsString())
		assert.Equal(t, "terraform", assumeRole["session_name"].AsString())
		assert.Equal(t, "1h", assumeRole["duration"].AsString())

		// Check nested tags
		tags := assumeRole["tags"].AsValueMap()
		assert.Equal(t, "prod", tags["Environment"].AsString())
		assert.Equal(t, "platform", tags["Team"].AsString())

		// Check slice
		accountIds := valueMap["allowed_account_ids"].AsValueSlice()
		assert.Len(t, accountIds, 2)
		assert.Equal(t, "123456", accountIds[0].AsString())
		assert.Equal(t, "234567", accountIds[1].AsString())
	})

	t.Run("converts unsupported type to cty.NilVal", func(t *testing.T) {
		// Test with a custom struct (unsupported type)
		type CustomStruct struct {
			Field string
		}

		input := CustomStruct{Field: "test"}
		result := GoToCty(input)

		assert.True(t, result.IsNull(), "unsupported type should convert to cty.NilVal")
	})

	t.Run("converts map with nil values", func(t *testing.T) {
		input := map[string]any{
			"key1": "value1",
			"key2": nil,
			"key3": "value3",
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		assert.Equal(t, "value1", valueMap["key1"].AsString())
		assert.True(t, valueMap["key2"].IsNull(), "nil value should be cty.NilVal")
		assert.Equal(t, "value3", valueMap["key3"].AsString())
	})

	t.Run("converts slice with nil values", func(t *testing.T) {
		input := []any{"value1", nil, "value3"}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 3)
		assert.Equal(t, "value1", valueSlice[0].AsString())
		assert.True(t, valueSlice[1].IsNull(), "nil value should be cty.NilVal")
		assert.Equal(t, "value3", valueSlice[2].AsString())
	})
}

// TestGoToCtyRoundTrip tests converting to cty and back to Go types.
func TestGoToCtyRoundTrip(t *testing.T) {
	t.Run("round trip simple types", func(t *testing.T) {
		testCases := []struct {
			name  string
			input any
		}{
			{"string", "hello"},
			{"int", 42},
			{"bool true", true},
			{"bool false", false},
			{"float", 3.14},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctyVal := GoToCty(tc.input)
				goVal := CtyToGo(ctyVal)
				// Note: int converts to int64 through cty
				// Note: float64 converts to int64 through cty (loses decimal precision)
				if _, ok := tc.input.(int); ok {
					assert.Equal(t, int64(tc.input.(int)), goVal)
				} else if f, ok := tc.input.(float64); ok {
					// CtyToGo always returns int64 for numbers, losing decimal precision
					assert.Equal(t, int64(f), goVal)
				} else {
					assert.Equal(t, tc.input, goVal)
				}
			})
		}
	})

	t.Run("round trip map", func(t *testing.T) {
		input := map[string]any{
			"name":  "test",
			"count": int64(42), // Use int64 to match what CtyToGo returns
			"flag":  true,
		}

		ctyVal := GoToCty(input)
		goVal := CtyToGo(ctyVal)

		result, ok := goVal.(map[string]any)
		require.True(t, ok, "should convert back to map")
		assert.Equal(t, input, result)
	})

	t.Run("round trip slice", func(t *testing.T) {
		input := []any{"one", "two", "three"}

		ctyVal := GoToCty(input)
		goVal := CtyToGo(ctyVal)

		result, ok := goVal.([]any)
		require.True(t, ok, "should convert back to slice")
		assert.Equal(t, input, result)
	})

	t.Run("round trip nested structure", func(t *testing.T) {
		input := map[string]any{
			"outer": map[string]any{
				"inner": []any{"a", "b", "c"},
			},
		}

		ctyVal := GoToCty(input)
		goVal := CtyToGo(ctyVal)

		result, ok := goVal.(map[string]any)
		require.True(t, ok, "should convert back to map")

		outer, ok := result["outer"].(map[string]any)
		require.True(t, ok, "outer should be map")

		inner, ok := outer["inner"].([]any)
		require.True(t, ok, "inner should be slice")
		assert.Equal(t, []any{"a", "b", "c"}, inner)
	})
}

// TestGoToCtyEdgeCases tests edge cases and boundary conditions.
func TestGoToCtyEdgeCases(t *testing.T) {
	t.Run("handles large numbers", func(t *testing.T) {
		largeInt := int64(9223372036854775807) // max int64
		result := GoToCty(largeInt)

		bigFloat := result.AsBigFloat()
		val, _ := bigFloat.Int64()
		assert.Equal(t, largeInt, val)
	})

	t.Run("handles very nested maps", func(t *testing.T) {
		// Create a deeply nested structure
		input := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": "deep value",
						},
					},
				},
			},
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		// Navigate to the deepest level
		l1 := result.AsValueMap()["level1"].AsValueMap()
		l2 := l1["level2"].AsValueMap()
		l3 := l2["level3"].AsValueMap()
		l4 := l3["level4"].AsValueMap()
		deepValue := l4["level5"].AsString()

		assert.Equal(t, "deep value", deepValue)
	})

	t.Run("handles map with special characters in keys", func(t *testing.T) {
		input := map[string]any{
			"key-with-dashes":      "value1",
			"key_with_underscores": "value2",
			"key.with.dots":        "value3",
			"key:with:colons":      "value4",
		}

		result := GoToCty(input)
		require.True(t, result.Type().IsObjectType(), "should be object type")

		valueMap := result.AsValueMap()
		assert.Equal(t, "value1", valueMap["key-with-dashes"].AsString())
		assert.Equal(t, "value2", valueMap["key_with_underscores"].AsString())
		assert.Equal(t, "value3", valueMap["key.with.dots"].AsString())
		assert.Equal(t, "value4", valueMap["key:with:colons"].AsString())
	})

	t.Run("handles slice with single element", func(t *testing.T) {
		input := []any{"single"}

		result := GoToCty(input)
		require.True(t, result.Type().IsTupleType(), "should be tuple type")

		valueSlice := result.AsValueSlice()
		assert.Len(t, valueSlice, 1)
		assert.Equal(t, "single", valueSlice[0].AsString())
	})

	t.Run("handles zero values", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    any
			expected any
		}{
			{"zero int", 0, int64(0)},
			{"zero float", 0.0, int64(0)}, // CtyToGo converts all numbers to int64
			{"empty string", "", ""},
			{"false bool", false, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctyVal := GoToCty(tc.input)
				goVal := CtyToGo(ctyVal)
				assert.Equal(t, tc.expected, goVal)
			})
		}
	})
}
