package plandiff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitive(t *testing.T) {
	// Test sensitive value
	sensitive := map[string]interface{}{
		"sensitive": true,
		"value":     "secret",
	}
	assert.True(t, isSensitive(sensitive))

	// Test non-sensitive value
	nonSensitive := map[string]interface{}{
		"sensitive": false,
		"value":     "public",
	}
	assert.False(t, isSensitive(nonSensitive))

	// Test non-map value
	assert.False(t, isSensitive("string"))
	assert.False(t, isSensitive(123))
	assert.False(t, isSensitive(nil))
}

func TestFormatValue(t *testing.T) {
	// Test sensitive value
	sensitive := map[string]interface{}{
		"sensitive": true,
		"value":     "secret",
	}
	assert.Equal(t, "(sensitive value)", formatValue(sensitive))

	// Test non-sensitive value
	assert.Equal(t, "public", formatValue("public"))
	assert.Equal(t, "123", formatValue(123))
	assert.Equal(t, "true", formatValue(true))

	// Test very long string
	longString := string(make([]byte, 500))
	for i := 0; i < 500; i++ {
		longString = longString[:i] + "a" + longString[i+1:]
	}
	formattedLong := formatValue(longString)
	assert.Less(t, len(formattedLong), 350, "Long string should be truncated")

	// Test small map formatting
	smallMap := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
	}
	formattedSmallMap := formatValue(smallMap)
	assert.Contains(t, formattedSmallMap, "key1: value1")
	assert.Contains(t, formattedSmallMap, "key2: value2")

	// Test large map formatting
	largeMap := map[string]interface{}{}
	for i := 0; i < 10; i++ {
		largeMap[string(rune('a'+i))] = i
	}
	formattedLargeMap := formatValue(largeMap)
	assert.Contains(t, formattedLargeMap, "{")
	assert.Contains(t, formattedLargeMap, "}")
	assert.Contains(t, formattedLargeMap, "a: 0")
}

// TestFormatMapForDisplay tests the formatMapForDisplay function specifically.
func TestFormatMapForDisplay(t *testing.T) {
	// Test small map (3 or fewer entries) - should use compact representation
	smallMap := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	formattedSmallMap := formatMapForDisplay(smallMap)
	assert.Contains(t, formattedSmallMap, "key1: value1")
	assert.Contains(t, formattedSmallMap, "key2: value2")
	assert.Contains(t, formattedSmallMap, "key3: value3")
	// Small maps are formatted as {key1: value1, key2: value2, key3: value3} with no newlines
	assert.Equal(t, 0, strings.Count(formattedSmallMap, "\n"), "Small map should be formatted in a compact way without newlines")

	// Test larger map (more than 3 entries) - should use structured representation
	largeMap := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
	}
	formattedLargeMap := formatMapForDisplay(largeMap)
	assert.Contains(t, formattedLargeMap, "key1: value1")
	assert.Contains(t, formattedLargeMap, "key2: value2")
	assert.Contains(t, formattedLargeMap, "key3: value3")
	assert.Contains(t, formattedLargeMap, "key4: value4")
	assert.True(t, strings.Count(formattedLargeMap, "\n") >= 4, "Large map should be formatted with multiple lines")

	// Test nested map
	nestedMap := map[string]interface{}{
		"outer1": "value1",
		"outer2": map[string]interface{}{
			"inner1": "innerValue1",
			"inner2": "innerValue2",
		},
		"outer3": "value3",
		"outer4": "value4",
	}
	formattedNestedMap := formatMapForDisplay(nestedMap)
	assert.Contains(t, formattedNestedMap, "outer1: value1")
	assert.Contains(t, formattedNestedMap, "outer2: ")
	assert.Contains(t, formattedNestedMap, "inner1: innerValue1")
	assert.Contains(t, formattedNestedMap, "inner2: innerValue2")
	assert.True(t, strings.Count(formattedNestedMap, "\n") >= 4, "Nested map should be formatted with multiple lines")

	// Test map that looks like response headers
	headersMap := map[string]interface{}{
		"Access-Control-Allow-Origin": "*",
		"Content-Length":              "329",
		"Content-Type":                "text/plain; charset=utf-8",
		"Date":                        "Thu, 13 Mar 2025 18:01:38 GMT",
	}
	formattedHeadersMap := formatMapForDisplay(headersMap)
	assert.Contains(t, formattedHeadersMap, "Access-Control-Allow-Origin: *")
	assert.Contains(t, formattedHeadersMap, "Content-Length: 329")
	assert.Contains(t, formattedHeadersMap, "Content-Type: text/plain; charset=utf-8")
	assert.Contains(t, formattedHeadersMap, "Date: Thu, 13 Mar 2025 18:01:38 GMT")
	assert.True(t, strings.Count(formattedHeadersMap, "\n") >= 4, "Headers should be formatted with multiple lines")
}

// TestFormatMapDiff tests the formatMapDiff function specifically.
func TestFormatMapDiff(t *testing.T) {
	// Test case 1: Identical maps should show no changes
	map1 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	result := formatMapDiff(map1, map1)
	assert.Equal(t, "(no changes)", result, "Identical maps should show no changes")

	// Test case 2: Added keys in a larger map
	map2 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
		"key4": "value4",
		"key5": "value5",
	}

	result = formatMapDiff(map1, map2)
	// For larger maps, check the multi-line format with proper indentation
	// The output will be like:
	// {
	//     + key4: value4
	//     + key5: value5
	// }
	assert.Contains(t, result, "+ key4", "Should show added key with + prefix")
	assert.Contains(t, result, "+ key5", "Should show multiple added keys")
	assert.NotContains(t, result, "key1", "Unchanged keys should not be shown")
	assert.NotContains(t, result, "key2", "Unchanged keys should not be shown")
	assert.NotContains(t, result, "key3", "Unchanged keys should not be shown")

	// Test case 3: Single change in a small map
	smallMap1 := map[string]interface{}{
		"a": 1,
		"b": 2,
	}

	smallMap2 := map[string]interface{}{
		"a": 1,
		"b": 3,
	}

	result = formatMapDiff(smallMap1, smallMap2)
	// For small maps with only one change, we get a compact representation
	assert.Equal(t, "{~b: 2 => 3}", result, "Small map with one change should be compact")

	// Test case 4: Multiple changes in a larger map
	map4 := map[string]interface{}{
		"key1": "value1",
		"key2": "changed value",
		"key4": "value4",
		"key5": "value5",
		// key3 is deleted
	}

	result = formatMapDiff(map1, map4)
	// For multi-line results:
	assert.Contains(t, result, "~ key2: value2 => changed value", "Should show changed value")
	assert.Contains(t, result, "+ key4: value4", "Should show added key")
	assert.Contains(t, result, "+ key5: value5", "Should show added key")
	assert.Contains(t, result, "- key3: value3", "Should show deleted key")
	assert.NotContains(t, result, "key1", "Unchanged keys should not be shown")

	// Test case 5: Nested maps
	nestedMap1 := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"inner1": "innerValue1",
			"inner2": "innerValue2",
		},
	}

	nestedMap2 := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"inner1": "innerValue1",
			"inner2": "changed inner value",
			"inner3": "new inner value",
		},
	}

	result = formatMapDiff(nestedMap1, nestedMap2)
	// For nested maps, check the single-line representation of the nested change
	assert.Contains(t, result, "~nested:", "Should show nested map is changed")
	assert.Contains(t, result, "inner1: innerValue1", "Should include inner value in format")
	assert.Contains(t, result, "inner2: innerValue2", "Should include original inner value")
	assert.Contains(t, result, "inner2: changed inner value", "Should include changed inner value")
	assert.Contains(t, result, "inner3: new inner value", "Should include new inner value")

	// Test case 6: Empty maps
	emptyMap := map[string]interface{}{}
	nonEmptyMap := map[string]interface{}{
		"key": "value",
	}

	result = formatMapDiff(emptyMap, nonEmptyMap)
	assert.Equal(t, "{+key: value}", result, "Should show added key when comparing empty map")

	result = formatMapDiff(nonEmptyMap, emptyMap)
	assert.Equal(t, "{-key: value}", result, "Should show deleted key when comparing to empty map")

	// Test case 7: Response headers map (common use case)
	headersMap1 := map[string]interface{}{
		"Content-Type":   "application/json",
		"Content-Length": "100",
		"Date":           "Mon, 01 Jan 2023 12:00:00 GMT",
	}

	headersMap2 := map[string]interface{}{
		"Content-Type":   "application/json",
		"Content-Length": "200",
		"Date":           "Tue, 02 Jan 2023 12:00:00 GMT",
	}

	result = formatMapDiff(headersMap1, headersMap2)
	// With our test data, this map will have a compact representation
	assert.Contains(t, result, "~Content-Length: 100 => 200", "Should show changed header value")
	assert.Contains(t, result, "~Date: Mon, 01 Jan 2023 12:00:00 GMT => Tue, 02 Jan 2023 12:00:00 GMT", "Should show changed date")
	assert.NotContains(t, result, "Content-Type", "Unchanged header should not be shown")
}

// TestPrintAttributeDiff tests the printAttributeDiff function with maps.
func TestPrintAttributeDiff(t *testing.T) {
	// Test how printAttributeDiff handles maps
	var diff strings.Builder

	// Case 1: Two maps with differences
	map1 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	map2 := map[string]interface{}{
		"key1": "value1",
		"key2": "changed",
		"key4": "value4",
	}

	printAttributeDiff(&diff, "test_attr", map1, map2)
	result := diff.String()
	assert.Contains(t, result, "~ test_attr: {", "Should show attribute name with change symbol")
	assert.Contains(t, result, "~ key2: value2 => changed", "Should show changed key")
	assert.Contains(t, result, "+ key4: value4", "Should show added key")
	assert.Contains(t, result, "- key3: value3", "Should show deleted key")
	assert.NotContains(t, result, "key1", "Unchanged key should not be shown")

	// Reset the diff builder
	diff = strings.Builder{}

	// Case 2: Non-map values
	printAttributeDiff(&diff, "test_attr", "old", "new")
	result = diff.String()
	assert.Equal(t, "  ~ test_attr: old => new\n", result, "Should handle non-map values properly")

	// Reset the diff builder
	diff = strings.Builder{}

	// Case 3: Sensitive values
	sensitive1 := map[string]interface{}{"sensitive": true, "value": "secret1"}
	sensitive2 := map[string]interface{}{"sensitive": true, "value": "secret2"}

	printAttributeDiff(&diff, "test_sensitive", sensitive1, sensitive2)
	result = diff.String()
	assert.Equal(t, "  ~ test_sensitive: (sensitive value) => (sensitive value)\n", result, "Should handle sensitive values properly")
}

// TestFormatOutputChange tests the formatOutputChange function.
func TestFormatOutputChange(t *testing.T) {
	t.Run("both_sensitive", func(t *testing.T) {
		origValue := map[string]interface{}{"sensitive": true, "value": "secret1"}
		newValue := map[string]interface{}{"sensitive": true, "value": "secret2"}
		result := formatOutputChange("api_key", origValue, newValue)
		assert.Equal(t, "~ api_key: (sensitive value) => (sensitive value)\n", result)
	})

	t.Run("orig_sensitive", func(t *testing.T) {
		origValue := map[string]interface{}{"sensitive": true, "value": "secret"}
		newValue := "public_value"
		result := formatOutputChange("api_key", origValue, newValue)
		assert.Equal(t, "~ api_key: (sensitive value) => public_value\n", result)
	})

	t.Run("new_sensitive", func(t *testing.T) {
		origValue := "public_value"
		newValue := map[string]interface{}{"sensitive": true, "value": "secret"}
		result := formatOutputChange("api_key", origValue, newValue)
		assert.Equal(t, "~ api_key: public_value => (sensitive value)\n", result)
	})

	t.Run("neither_sensitive", func(t *testing.T) {
		result := formatOutputChange("url", "http://old.com", "http://new.com")
		assert.Equal(t, "~ url: http://old.com => http://new.com\n", result)
	})
}

// TestFormatStringValue tests the formatStringValue function.
func TestFormatStringValue(t *testing.T) {
	t.Run("weather_report", func(t *testing.T) {
		result := formatStringValue("Weather report: sunny")
		assert.Equal(t, "Weather report: sunny", result)
	})

	t.Run("base64_value_V2VhdGhl", func(t *testing.T) {
		result := formatStringValue("V2VhdGhlciBzb21ldGhpbmc=")
		assert.Equal(t, "(base64 encoded value)", result)
	})

	t.Run("base64_value_CgogIBtb", func(t *testing.T) {
		result := formatStringValue("CgogIBtbMDszOG0gc29tZXRoaW5n")
		assert.Equal(t, "(base64 encoded value)", result)
	})

	t.Run("normal_string", func(t *testing.T) {
		result := formatStringValue("normal string")
		assert.Equal(t, "normal string", result)
	})

	t.Run("long_string_truncated", func(t *testing.T) {
		longString := strings.Repeat("a", 500)
		result := formatStringValue(longString)
		assert.Less(t, len(result), 500)
		assert.Contains(t, result, "...")
	})
}

// TestDebugFormatMapDiff outputs the exact diff format for debugging.
func TestDebugFormatMapDiff(t *testing.T) {
	// Simple map diff
	map1 := map[string]interface{}{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	map2 := map[string]interface{}{
		"key1": "value1",
		"key2": "changed value",
		"key4": "value4",
	}

	result := formatMapDiff(map1, map2)
	t.Logf("Simple map diff result: %q", result)

	// Verify the result is not empty and contains expected markers.
	assert.NotEmpty(t, result, "formatMapDiff should return non-empty string for different maps")
	assert.Contains(t, result, "key2", "Result should contain changed key")

	// Nested map diff
	nestedMap1 := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"inner1": "innerValue1",
			"inner2": "innerValue2",
		},
	}

	nestedMap2 := map[string]interface{}{
		"key1": "value1",
		"nested": map[string]interface{}{
			"inner1": "innerValue1",
			"inner2": "changed inner value",
			"inner3": "new inner value",
		},
	}

	result = formatMapDiff(nestedMap1, nestedMap2)
	t.Logf("Nested map diff result: %q", result)

	// Verify nested diffs are captured.
	assert.NotEmpty(t, result, "formatMapDiff should return non-empty string for nested diffs")
	assert.Contains(t, result, "inner2", "Result should contain changed nested key")

	// Small map diff
	smallMap1 := map[string]interface{}{
		"a": 1,
		"b": 2,
	}

	smallMap2 := map[string]interface{}{
		"a": 1,
		"b": 3,
	}

	result = formatMapDiff(smallMap1, smallMap2)
	t.Logf("Small map diff result: %q", result)

	// Empty map diff
	emptyMap := map[string]interface{}{}
	nonEmptyMap := map[string]interface{}{
		"key": "value",
	}

	result = formatMapDiff(emptyMap, nonEmptyMap)
	t.Logf("Empty map diff result: %q", result)
}
