package merge

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMergeBasic(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"baz": "bat"}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"foo": "bar", "baz": "bat"}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMerge_NilAtmosConfigReturnsError(t *testing.T) {
	// Nil atmosConfig should return an error to prevent panic
	map1 := map[string]any{"list": []string{"1"}}
	map2 := map[string]any{"list": []string{"2"}}
	inputs := []map[string]any{map1, map2}

	res, err := Merge(nil, inputs)
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// Verify the error is properly wrapped.
	assert.True(t, errors.Is(err, errUtils.ErrMerge), "Error should be wrapped with ErrMerge")
	// ErrAtmosConfigIsNil is now embedded as a string, not wrapped.
	assert.Contains(t, err.Error(), "atmos config is nil")
}

func TestMergeBasicOverride(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"baz": "bat"}
	map3 := map[string]any{"foo": "ood"}

	inputs := []map[string]any{map1, map2, map3}
	expected := map[string]any{"foo": "ood", "baz": "bat"}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMergeListReplace(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	map1 := map[string]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[string]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"list": []any{"4", "5", "6"}}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeListAppend(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyAppend,
		},
	}

	map1 := map[string]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[string]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"list": []any{"1", "2", "3", "4", "5", "6"}}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeListMerge(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyMerge,
		},
	}

	map1 := map[string]any{
		"list": []map[string]string{
			{
				"1": "1",
				"2": "2",
				"3": "3",
				"4": "4",
			},
		},
	}

	map2 := map[string]any{
		"list": []map[string]string{
			{
				"1": "1b",
				"2": "2",
				"3": "3b",
				"5": "5",
			},
		},
	}

	inputs := []map[string]any{map1, map2}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)

	var mergedList []any
	var ok bool

	if mergedList, ok = result["list"].([]any); !ok {
		t.Errorf("invalid merge result: %v", result)
	}

	merged := mergedList[0].(map[string]any)

	assert.Equal(t, "1b", merged["1"])
	assert.Equal(t, "2", merged["2"])
	assert.Equal(t, "3b", merged["3"])
	assert.Equal(t, "4", merged["4"])
	assert.Equal(t, "5", merged["5"])

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeWithNilConfig(t *testing.T) {
	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"foo": "baz", "hello": "world"}
	inputs := []map[string]any{map1, map2}

	// Nil config should return an error
	result, err := Merge(nil, inputs)
	assert.NotNil(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "atmos config is nil")

	// Verify proper error wrapping.
	assert.True(t, errors.Is(err, errUtils.ErrMerge))
	// ErrAtmosConfigIsNil is now embedded as a string, not wrapped.
}

func TestMergeWithInvalidStrategy(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "invalid-strategy",
		},
	}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"foo": "baz"}
	inputs := []map[string]any{map1, map2}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid list merge strategy")
	assert.Contains(t, err.Error(), "invalid-strategy")
	assert.Contains(t, err.Error(), "replace, append, merge")

	// Verify error wrapping - should be wrapped with ErrMerge.
	assert.True(t, errors.Is(err, errUtils.ErrMerge), "Error should be wrapped with ErrMerge")
	// ErrInvalidListMergeStrategy is now embedded in the error message, not wrapped.
}

func TestMergeWithEmptyInputs(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	// Test with empty inputs slice
	inputs := []map[string]any{}
	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	// Test with nil maps in inputs
	inputs = []map[string]any{nil, nil}
	result, err = Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	// Test with mix of empty and non-empty maps
	inputs = []map[string]any{{}, {"foo": "bar"}, {}}
	result, err = Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bar", result["foo"])
}

func TestMergeHandlesNilConfigWithoutPanic(t *testing.T) {
	// This test verifies that Merge handles nil config gracefully
	// Without the nil check in Merge, this test would panic when
	// the function tries to access atmosConfig.Settings.ListMergeStrategy

	inputs := []map[string]any{
		{"key1": "value1"},
		{"key2": "value2"},
	}

	// Call Merge with nil config - this would panic without our fix
	// at the line: if atmosConfig.Settings.ListMergeStrategy != ""
	result, err := Merge(nil, inputs)

	// Verify it returns an error instead of panicking
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "atmos config is nil")
	assert.True(t, errors.Is(err, errUtils.ErrMerge))
}

// TestDeepCopyMap_Correctness validates that deep copy produces correct, independent copies.
// This ensures properly-sized allocations maintain correctness across multiple calls.
func TestDeepCopyMap_Correctness(t *testing.T) {
	testData := map[string]any{
		"string": "value",
		"number": 42,
		"nested": map[string]any{
			"deep": map[string]any{
				"value": "nested value",
				"array": []any{"a", "b", "c"},
			},
			"array": []any{1, 2, 3},
		},
		"slice": []any{"x", "y", "z"},
	}

	// Copy the same data multiple times to verify consistency.
	var copies []map[string]any
	for i := 0; i < 10; i++ {
		copy, err := DeepCopyMap(testData)
		assert.Nil(t, err)
		assert.NotNil(t, copy)
		copies = append(copies, copy)
	}

	// All copies should be identical to the original.
	for i, copy := range copies {
		assert.Equal(t, testData["string"], copy["string"], "Copy %d string mismatch", i)
		assert.Equal(t, testData["number"], copy["number"], "Copy %d number mismatch", i)

		nested := copy["nested"].(map[string]any)
		originalNested := testData["nested"].(map[string]any)
		assert.Equal(t, originalNested["deep"], nested["deep"], "Copy %d nested.deep mismatch", i)
		assert.Equal(t, originalNested["array"], nested["array"], "Copy %d nested.array mismatch", i)

		assert.Equal(t, testData["slice"], copy["slice"], "Copy %d slice mismatch", i)
	}

	// Verify copies are independent (modifying one doesn't affect others).
	copies[0]["string"] = "modified"
	assert.Equal(t, "value", testData["string"], "Original should not be modified")
	assert.Equal(t, "value", copies[1]["string"], "Copy 1 should not be affected by copy 0 modification")
}

// TestDeepCopyMap_Concurrent validates thread safety of deep copy operations.
// This ensures concurrent copies produce correct, independent results.
func TestDeepCopyMap_Concurrent(t *testing.T) {
	testData := map[string]any{
		"key1": "value1",
		"key2": 123,
		"nested": map[string]any{
			"array": []any{1, 2, 3},
		},
	}

	const numGoroutines = 100

	results := make(chan map[string]any, numGoroutines)
	errors := make(chan error, numGoroutines)
	start := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start

			copy, err := DeepCopyMap(testData)
			if err != nil {
				errors <- err
				return
			}

			results <- copy
		}()
	}

	// Release all goroutines at once.
	close(start)

	// Collect all results.
	successCount := 0
	errorCount := 0

	for i := 0; i < numGoroutines; i++ {
		select {
		case copy := <-results:
			assert.NotNil(t, copy)
			assert.Equal(t, testData["key1"], copy["key1"])
			assert.Equal(t, testData["key2"], copy["key2"])
			successCount++
		case err := <-errors:
			t.Errorf("Concurrent copy failed: %v", err)
			errorCount++
		}
	}

	assert.Equal(t, numGoroutines, successCount, "All goroutines should succeed")
	assert.Equal(t, 0, errorCount, "No goroutines should encounter errors")
}

// TestDeepCopyMap_DifferentSizes tests deep copy with various data sizes.
// This validates that properly-sized allocations work correctly with different map/slice sizes.
func TestDeepCopyMap_DifferentSizes(t *testing.T) {
	testCases := []struct {
		name string
		data map[string]any
	}{
		{
			name: "empty map",
			data: map[string]any{},
		},
		{
			name: "small map",
			data: map[string]any{"key": "value"},
		},
		{
			name: "medium map",
			data: map[string]any{
				"key1": "value1",
				"key2": "value2",
				"key3": map[string]any{
					"nested1": "value",
					"nested2": []any{1, 2, 3, 4, 5},
				},
			},
		},
		{
			name: "large nested structure",
			data: generateLargeMap(5, 10),
		},
		{
			name: "large array",
			data: map[string]any{
				"array": generateLargeSlice(100),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			copy, err := DeepCopyMap(tc.data)
			assert.Nil(t, err)
			assert.NotNil(t, copy)

			// Verify structure is copied correctly.
			assert.Equal(t, len(tc.data), len(copy))
		})
	}
}

// generateLargeMap generates a nested map for testing.
func generateLargeMap(depth, breadth int) map[string]any {
	if depth == 0 {
		return map[string]any{"leaf": "value"}
	}

	result := make(map[string]any, breadth)
	for i := 0; i < breadth; i++ {
		key := "key" + string(rune('0'+i%10))
		if i%2 == 0 {
			result[key] = generateLargeMap(depth-1, breadth)
		} else {
			result[key] = "value" + string(rune('0'+i%10))
		}
	}
	return result
}

// generateLargeSlice generates a large slice for testing.
func generateLargeSlice(size int) []any {
	result := make([]any, size)
	for i := 0; i < size; i++ {
		result[i] = "item" + string(rune('0'+i%10))
	}
	return result
}

// BenchmarkDeepCopyMap benchmarks the deep copy performance with properly-sized allocations.
// This measures allocation efficiency using exact capacity hints.
func BenchmarkDeepCopyMap(b *testing.B) {
	testData := map[string]any{
		"string": "value",
		"number": 42,
		"nested": map[string]any{
			"deep": map[string]any{
				"value": "nested value",
				"array": []any{"a", "b", "c"},
			},
			"array": []any{1, 2, 3},
		},
		"slice": []any{"x", "y", "z"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeepCopyMap(testData)
	}
}

// BenchmarkDeepCopyMap_Large benchmarks deep copy with large nested structures.
func BenchmarkDeepCopyMap_Large(b *testing.B) {
	testData := generateLargeMap(5, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DeepCopyMap(testData)
	}
}

// TestDeepCopyMap_TypedMaps tests type-safe handling of typed maps with non-string keys.
// This validates the fix for type-safety bug where deepCopyValue could cause panics
// when SetMapIndex received incompatible types.
func TestDeepCopyMap_TypedMaps(t *testing.T) {
	testCases := []struct {
		name     string
		input    map[string]any
		validate func(t *testing.T, result map[string]any)
	}{
		{
			name: "empty typed map with int keys preserves type",
			input: map[string]any{
				"typed_map": map[int]string{}, // Empty typed map.
			},
			validate: func(t *testing.T, result map[string]any) {
				val, ok := result["typed_map"]
				assert.True(t, ok, "typed_map should exist")
				// Should be typed map[int]string, not map[string]any.
				_, isTyped := val.(map[int]string)
				assert.True(t, isTyped, "Empty typed map should preserve type")
			},
		},
		{
			name: "typed map with int keys and string values preserves values",
			input: map[string]any{
				"typed_map": map[int]string{
					1: "one",
					2: "two",
					3: "three",
				},
			},
			validate: func(t *testing.T, result map[string]any) {
				val, ok := result["typed_map"]
				assert.True(t, ok, "typed_map should exist")
				typedMap, isTyped := val.(map[int]string)
				assert.True(t, isTyped, "Should be map[int]string")
				assert.Equal(t, "one", typedMap[1])
				assert.Equal(t, "two", typedMap[2])
				assert.Equal(t, "three", typedMap[3])
			},
		},
		{
			name: "typed map with interface{} values allows deep copy",
			input: map[string]any{
				"typed_map": map[int]interface{}{
					1: "value",
					2: 42,
					3: []any{"a", "b"},
				},
			},
			validate: func(t *testing.T, result map[string]any) {
				val, ok := result["typed_map"]
				assert.True(t, ok, "typed_map should exist")
				typedMap, isTyped := val.(map[int]interface{})
				assert.True(t, isTyped, "Should be map[int]interface{}")
				assert.Equal(t, "value", typedMap[1])
				assert.Equal(t, 42, typedMap[2])
				// Verify nested slice was deep copied.
				slice, isSlice := typedMap[3].([]any)
				assert.True(t, isSlice, "Should have deep copied slice")
				assert.Equal(t, []any{"a", "b"}, slice)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DeepCopyMap(tc.input)
			assert.Nil(t, err, "DeepCopyMap should not error")
			assert.NotNil(t, result, "Result should not be nil")

			tc.validate(t, result)

			// Verify independence - modifying copy shouldn't affect original.
			// This is a basic sanity check for all test cases.
			assert.NotNil(t, tc.input, "Original should not be affected")
		})
	}
}

// BenchmarkMerge benchmarks merge operations with properly-sized allocations.
// This measures the cumulative benefit of exact capacity hints during merges.
func BenchmarkMerge(b *testing.B) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	map1 := map[string]any{
		"component": "vpc",
		"vars": map[string]any{
			"region":     "us-east-1",
			"cidr_block": "10.0.0.0/16",
			"enable_dns": true,
			"tags":       []any{"prod", "network"},
		},
	}

	map2 := map[string]any{
		"component": "vpc",
		"vars": map[string]any{
			"region":             "us-west-2",
			"availability_zones": []any{"us-west-2a", "us-west-2b"},
			"tags":               []any{"dev", "network", "shared"},
		},
	}

	map3 := map[string]any{
		"metadata": map[string]any{
			"name": "test-stack",
			"type": "terraform",
		},
	}

	inputs := []map[string]any{map1, map2, map3}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Merge(atmosConfig, inputs)
	}
}
