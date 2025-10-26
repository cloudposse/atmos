package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSortMapKeys(t *testing.T) {
	// Test with a simple map
	input := map[string]interface{}{
		"c": 3,
		"a": 1,
		"b": 2,
	}

	expected := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}

	result := sortMapKeys(input)
	assert.Equal(t, expected, result)

	// Test with nested maps
	nestedInput := map[string]interface{}{
		"z": map[string]interface{}{
			"y": 2,
			"x": 1,
		},
		"a": 1,
	}

	nestedExpected := map[string]interface{}{
		"a": 1,
		"z": map[string]interface{}{
			"x": 1,
			"y": 2,
		},
	}

	nestedResult := sortMapKeys(nestedInput)
	assert.Equal(t, nestedExpected, nestedResult)

	// Test with arrays of maps
	arrayInput := map[string]interface{}{
		"arr": []interface{}{
			map[string]interface{}{
				"c": 3,
				"a": 1,
			},
			map[string]interface{}{
				"z": 26,
				"y": 25,
			},
		},
	}

	arrayExpected := map[string]interface{}{
		"arr": []interface{}{
			map[string]interface{}{
				"a": 1,
				"c": 3,
			},
			map[string]interface{}{
				"y": 25,
				"z": 26,
			},
		},
	}

	arrayResult := sortMapKeys(arrayInput)
	assert.Equal(t, arrayExpected, arrayResult)
}

func TestGetVariables(t *testing.T) {
	plan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
			"stage": map[string]interface{}{
				"value": "dev",
			},
		},
	}

	expected := map[string]interface{}{
		"location": "Stockholm",
		"stage":    "dev",
	}

	result := getVariables(plan)
	assert.Equal(t, expected, result)
}

func TestGetOutputs(t *testing.T) {
	plan := map[string]interface{}{
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
		},
		"output_changes": map[string]interface{}{
			"location": map[string]interface{}{
				"actions":          []interface{}{"create"},
				"before":           nil,
				"after":            "Stockholm",
				"after_unknown":    false,
				"before_sensitive": false,
				"after_sensitive":  false,
			},
		},
	}

	expected := map[string]interface{}{
		"url": map[string]interface{}{
			"sensitive": false,
			"value":     "https://example.com",
		},
		"location": map[string]interface{}{
			"actions":          []interface{}{"create"},
			"before":           nil,
			"after":            "Stockholm",
			"after_unknown":    false,
			"before_sensitive": false,
			"after_sensitive":  false,
		},
	}

	result := getOutputs(plan)
	assert.Equal(t, expected, result)
}

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

func TestGeneratePlanDiff(t *testing.T) {
	// Test with identical plans
	origPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
			},
		},
	}

	diff, hasDiff := generatePlanDiff(origPlan, origPlan)
	assert.False(t, hasDiff)
	assert.Empty(t, diff)

	// Test with different plans
	newPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
			},
		},
	}

	diff, hasDiff = generatePlanDiff(origPlan, newPlan)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")

	// Test with sensitive output values (Terraform marks outputs as sensitive)
	sensitiveOutputPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
				"api_key": map[string]interface{}{
					"sensitive": true,
					"value":     "api_key_12345",
				},
			},
		},
	}

	newSensitiveOutputPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
				"api_key": map[string]interface{}{
					"sensitive": true,
					"value":     "api_key_67890",
				},
			},
		},
	}

	diff, hasDiff = generatePlanDiff(sensitiveOutputPlan, newSensitiveOutputPlan)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")
	assert.Contains(t, diff, "~ api_key: (sensitive value) => (sensitive value)")
	assert.NotContains(t, diff, "api_key_12345")
	assert.NotContains(t, diff, "api_key_67890")
}

func TestPlanDiffCommandFlags(t *testing.T) {
	// Create a test atmosphere configuration
	atmosConfig := schema.AtmosConfiguration{
		TerraformDirAbsolutePath: ".",
	}

	// Test with missing --orig flag
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "test-component",
		AdditionalArgsAndFlags: []string{
			"--new=/tmp/new.plan",
		},
	}

	err := TerraformPlanDiff(&atmosConfig, &info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "original plan file (--orig) is required")

	// Test with --orig flag but no --new flag (should use generated new plan)
	info = schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "test-component",
		AdditionalArgsAndFlags: []string{
			"--orig=/tmp/orig.plan",
		},
	}

	// We can't fully test this case without mocking terraform commands,
	// but we can at least check that it attempts to generate a new plan
	// The test will fail with a file not found error, which is expected
	err = TerraformPlanDiff(&atmosConfig, &info)
	assert.Error(t, err)
	// The error will be related to not finding the file or running terraform
}

func TestTerraformPlanDiffWithNonExistentFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test atmosConfig
	atmosConfig := schema.AtmosConfiguration{
		TerraformDirAbsolutePath: tmpDir,
	}

	// Create a component directory
	componentDir := filepath.Join(tmpDir, "test-component")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Test with non-existent original plan file using a relative path
	// This should be resolved relative to the component directory
	relPath := "non-existent.plan"
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "test-component",
		AdditionalArgsAndFlags: []string{
			"--orig=" + relPath,
		},
		SkipInit: true, // Skip init for test
	}

	err = TerraformPlanDiff(&atmosConfig, &info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	// The error should mention the path relative to the component directory
	expectedNonExistentPath := filepath.Join(componentDir, relPath)
	assert.Contains(t, err.Error(), expectedNonExistentPath)

	// Test with non-existent original plan file using an absolute path
	// This should be used as-is
	absPath := filepath.Join(tmpDir, "another-non-existent.plan")
	info = schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "test-component",
		AdditionalArgsAndFlags: []string{
			"--orig=" + absPath,
		},
		SkipInit: true, // Skip init for test
	}

	err = TerraformPlanDiff(&atmosConfig, &info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), absPath)
}

// TestTerraformPlanDiffErrorHandling tests that the TerraformPlanDiff function returns the correct error types in
// different scenarios.
func TestTerraformPlanDiffErrorHandling(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create a test atmosConfig
	atmosConfig := schema.AtmosConfiguration{
		TerraformDirAbsolutePath: tmpDir,
	}

	// Create a component directory
	componentDir := filepath.Join(tmpDir, "test-component")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create test plan files
	origPlanFile := filepath.Join(tmpDir, "orig.plan")
	newPlanFile := filepath.Join(tmpDir, "new.plan")

	// Create empty files (will cause JSON parsing errors)
	err = os.WriteFile(origPlanFile, []byte{}, 0o644)
	require.NoError(t, err)
	err = os.WriteFile(newPlanFile, []byte{}, 0o644)
	require.NoError(t, err)

	// Test with empty plan files (should fail when trying to parse the JSON)
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "test-component",
		AdditionalArgsAndFlags: []string{
			"--orig=" + origPlanFile,
			"--new=" + newPlanFile,
		},
		SkipInit: true, // Skip init for test
	}

	// Save the original OsExit function and restore it after the test
	originalOsExit := errUtils.OsExit
	defer func() { errUtils.OsExit = originalOsExit }()

	// Mock OsExit to prevent the test from exiting
	exitCalled := false
	errUtils.OsExit = func(code int) {
		exitCalled = true
		// Don't actually exit
	}

	// This should return a regular error, not call OsExit
	err = TerraformPlanDiff(&atmosConfig, &info)
	assert.Error(t, err)
	assert.False(t, exitCalled, "OsExit should not be called for JSON parsing errors")
}

// TestMockTerraformPlanDiff tests the generatePlanDiff function directly.
func TestMockTerraformPlanDiff(t *testing.T) {
	// Create JSON content for the original plan
	origPlanJSON := map[string]interface{}{
		"format_version":    "1.2",
		"terraform_version": "1.5.7",
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
			},
		},
	}

	// Create JSON content for the new plan
	newPlanJSON := map[string]interface{}{
		"format_version":    "1.2",
		"terraform_version": "1.5.7",
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
			},
		},
	}

	// Test the generatePlanDiff function directly
	diff, hasDiff := generatePlanDiff(origPlanJSON, newPlanJSON)
	assert.True(t, hasDiff, "Plans should be different")
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")

	// Test with identical plans
	diff, hasDiff = generatePlanDiff(origPlanJSON, origPlanJSON)
	assert.False(t, hasDiff, "Identical plans should not have differences")
	assert.Empty(t, diff, "Diff should be empty for identical plans")
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
