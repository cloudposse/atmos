package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformPlanDiffProcessStacksError tests the error path when ProcessStacks fails.
func TestTerraformPlanDiffProcessStacksError(t *testing.T) {
	// Create a minimal atmosConfig
	atmosConfig := &schema.AtmosConfiguration{
		TerraformDirAbsolutePath: t.TempDir(),
	}

	// Test case 1: Missing stack triggers ProcessStacks error
	t.Run("missing_stack_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "", // Missing stack - will cause ProcessStacks to fail
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stack is required")
	})

	// Test case 2: Missing component triggers ProcessStacks error
	t.Run("missing_component_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "", // Missing component - will cause ProcessStacks to fail
			Stack:            "test-stack",
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "component")
	})

	// Test case 3: Invalid stack configuration triggers ProcessStacks error
	t.Run("invalid_stack_config_returns_error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "nonexistent-component",
			Stack:            "nonexistent-stack",
			ComponentType:    "terraform",
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// ProcessStacks will fail because it can't find the stack configuration
	})
}

// TestTerraformPlanDiffWithVariousInputs tests the TerraformPlanDiff function with various inputs.
// These tests exercise the function entry point and error handling paths.
func TestTerraformPlanDiffWithVariousInputs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create atmosConfig
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: filepath.Join(tmpDir, "components", "terraform"),
	}

	// Test with empty AdditionalArgsAndFlags - ensures function entry is covered
	t.Run("empty_args_with_valid_stack_component", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:       "test-component",
			Stack:                  "test-stack",
			ComponentType:          "terraform",
			AdditionalArgsAndFlags: []string{},
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// Will fail at ProcessStacks because stack config doesn't exist
	})

	// Test with flags but invalid stack - ensures function processes input
	t.Run("with_flags_invalid_stack", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "nonexistent-stack",
			ComponentType:    "terraform",
			AdditionalArgsAndFlags: []string{
				"--orig=test.planfile",
				"--new=new.planfile",
			},
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
		// Will fail at ProcessStacks
	})

	// Test with nil AdditionalArgsAndFlags
	t.Run("nil_args", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:       "test-component",
			Stack:                  "test-stack",
			ComponentType:          "terraform",
			AdditionalArgsAndFlags: nil,
		}

		err := TerraformPlanDiff(atmosConfig, info)
		assert.Error(t, err)
	})
}

// TestTerraformPlanDiffWithValidFixture tests the TerraformPlanDiff function with a valid fixture.
// This test exercises the code path AFTER ProcessStacks succeeds (covers the "happy path" branch).
func TestTerraformPlanDiffWithValidFixture(t *testing.T) {
	// Change to the test fixture directory
	testDir := "../../tests/fixtures/scenarios/atmos-terraform-plan-diff"
	t.Chdir(testDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Test 1: ProcessStacks succeeds, but --orig flag is missing
	t.Run("process_stacks_succeeds_missing_orig_flag", func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:       "base-component",
			Stack:                  "test-stack",
			ComponentType:          cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{}, // Missing --orig flag
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "original plan file (--orig) is required" AFTER ProcessStacks succeeds
		assert.Contains(t, err.Error(), "original plan file (--orig) is required")
	})

	// Test 2: ProcessStacks succeeds, --orig flag provided, but file doesn't exist
	t.Run("process_stacks_succeeds_nonexistent_planfile", func(t *testing.T) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:       "base-component",
			Stack:                  "test-stack",
			ComponentType:          cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{"--orig=nonexistent.planfile"},
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "does not exist" after ProcessStacks and parsePlanDiffFlags succeed
		assert.Contains(t, err.Error(), "does not exist")
	})

	// Test 3: ProcessStacks succeeds, planfile exists, but new planfile doesn't exist
	// This exercises more of the code path including prepareNewPlanFile
	t.Run("process_stacks_succeeds_with_existing_orig_planfile", func(t *testing.T) {
		// Create a dummy planfile in the component directory
		planfilePath := filepath.Join("components", "terraform", "base-component", "test.planfile")
		err := os.WriteFile(planfilePath, []byte("dummy plan data"), 0o644)
		require.NoError(t, err)
		defer os.Remove(planfilePath)

		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg: "base-component",
			Stack:            "test-stack",
			ComponentType:    cfg.TerraformComponentType,
			AdditionalArgsAndFlags: []string{
				"--orig=test.planfile",
				"--new=nonexistent-new.planfile", // New file doesn't exist
			},
			SkipInit: true, // Skip terraform init for this test
		}

		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		require.NoError(t, err)

		err = TerraformPlanDiff(&atmosConfig, &configAndStacksInfo)
		assert.Error(t, err)
		// Should fail with "does not exist" for the new planfile
		// This exercises validateOriginalPlanFile success and prepareNewPlanFile
		assert.Contains(t, err.Error(), "does not exist")
	})
}

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

func TestTerraformPlanDiffWithNonExistentFile(t *testing.T) {
	// Test validateOriginalPlanFile directly as a unit test
	tmpDir := t.TempDir()

	// Create a component directory
	componentDir := filepath.Join(tmpDir, "test-component")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Test with non-existent original plan file using a relative path
	// This should be resolved relative to the component directory
	relPath := "non-existent.plan"
	_, err = validateOriginalPlanFile(relPath, componentDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	// The error should mention the path relative to the component directory
	expectedNonExistentPath := filepath.Join(componentDir, relPath)
	assert.Contains(t, err.Error(), expectedNonExistentPath)

	// Test with non-existent original plan file using an absolute path
	// This should be used as-is
	absPath := filepath.Join(tmpDir, "another-non-existent.plan")
	_, err = validateOriginalPlanFile(absPath, componentDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Contains(t, err.Error(), absPath)

	// Test with an existing file
	existingFile := filepath.Join(componentDir, "existing.plan")
	err = os.WriteFile(existingFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Relative path should work
	result, err := validateOriginalPlanFile("existing.plan", componentDir)
	assert.NoError(t, err)
	assert.Equal(t, existingFile, result)

	// Absolute path should also work
	result, err = validateOriginalPlanFile(existingFile, componentDir)
	assert.NoError(t, err)
	assert.Equal(t, existingFile, result)
}

// TestTerraformPlanDiffErrorHandling tests the error handling in plan-diff helper functions.
func TestTerraformPlanDiffErrorHandling(t *testing.T) {
	// Test extractJSONFromOutput with empty output (should return ErrNoJSONOutput)
	t.Run("empty_output_returns_ErrNoJSONOutput", func(t *testing.T) {
		_, err := extractJSONFromOutput("")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoJSONOutput)
	})

	// Test extractJSONFromOutput with no JSON content
	t.Run("no_json_content_returns_ErrNoJSONOutput", func(t *testing.T) {
		_, err := extractJSONFromOutput("some random text without json")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNoJSONOutput)
	})

	// Test extractJSONFromOutput with valid JSON
	t.Run("valid_json_extracted", func(t *testing.T) {
		output := `Some prefix text
{"key": "value"}
trailing text`
		result, err := extractJSONFromOutput(output)
		assert.NoError(t, err)
		assert.Contains(t, result, `"key": "value"`)
	})

	// Test parsePlanDiffFlags error handling
	t.Run("missing_orig_flag_returns_error", func(t *testing.T) {
		_, _, err := parsePlanDiffFlags([]string{"--new=something.plan"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "original plan file (--orig) is required")
	})
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

// TestTerraformPlanDiffMetadataComponentResolution verifies that plan-diff uses FinalComponent
// (resolved from metadata.component) rather than ComponentFromArg when constructing
// the component path. This prevents searching the wrong directory (e.g.,
// components/terraform/foobar-atmos-pro/ instead of components/terraform/foobar/).
func TestTerraformPlanDiffMetadataComponentResolution(t *testing.T) {
	// This is a unit test for the component path construction logic.
	// It tests that the componentPath is correctly built from FinalComponent,
	// not from ComponentFromArg.

	tmpDir := t.TempDir()

	// Create the component directory structure:
	// - components/terraform/base-component/ (actual component code)
	// - components/terraform/derived-component/ should NOT exist
	baseComponentDir := filepath.Join(tmpDir, "components", "terraform", "base-component")
	err := os.MkdirAll(baseComponentDir, 0o755)
	require.NoError(t, err)

	// Create a dummy planfile in the CORRECT location (base-component)
	planfileName := "test.planfile"
	planfilePath := filepath.Join(baseComponentDir, planfileName)
	err = os.WriteFile(planfilePath, []byte("dummy plan data"), 0o644)
	require.NoError(t, err)

	terraformDirPath := filepath.Join(tmpDir, "components", "terraform")

	// Test that validateOriginalPlanFile finds the file in the correct component path
	// when FinalComponent is properly set (simulates what happens after ProcessStacks)
	t.Run("component_path_with_FinalComponent", func(t *testing.T) {
		// Construct component path as TerraformPlanDiff does after ProcessStacks
		componentFolderPrefix := ""
		finalComponent := "base-component" // Resolved from metadata.component
		componentPath := filepath.Join(terraformDirPath, componentFolderPrefix, finalComponent)

		// Validate that the planfile is found in the correct path
		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfilePath, result)
	})

	// Test that the planfile would NOT be found if we used ComponentFromArg instead of FinalComponent
	// This demonstrates what the bug was before the fix
	t.Run("component_path_with_wrong_component_name", func(t *testing.T) {
		// Construct component path incorrectly (using ComponentFromArg instead of FinalComponent)
		componentFolderPrefix := ""
		componentFromArg := "derived-component" // Wrong! Should use FinalComponent
		wrongComponentPath := filepath.Join(terraformDirPath, componentFolderPrefix, componentFromArg)

		// Validate that the planfile is NOT found in the wrong path
		_, err := validateOriginalPlanFile(planfileName, wrongComponentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}

// TestCopyPlanFileIfNeeded tests the copyPlanFileIfNeeded function.
func TestCopyPlanFileIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	componentDir := filepath.Join(tmpDir, "components", "terraform", "mycomponent")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create a plan file in the component directory
	planFileInComponent := filepath.Join(componentDir, "existing.plan")
	err = os.WriteFile(planFileInComponent, []byte("plan data"), 0o644)
	require.NoError(t, err)

	// Test 1: Plan file already in component directory - no copy needed
	t.Run("plan_file_in_component_dir", func(t *testing.T) {
		result, cleanup, err := copyPlanFileIfNeeded(planFileInComponent, componentDir)
		assert.NoError(t, err)
		assert.Nil(t, cleanup)
		assert.Equal(t, planFileInComponent, result)
	})

	// Test 2: Plan file outside component directory - copy needed
	t.Run("plan_file_outside_component_dir", func(t *testing.T) {
		externalPlanFile := filepath.Join(tmpDir, "external.plan")
		err := os.WriteFile(externalPlanFile, []byte("external plan data"), 0o644)
		require.NoError(t, err)

		result, cleanup, err := copyPlanFileIfNeeded(externalPlanFile, componentDir)
		assert.NoError(t, err)
		assert.NotNil(t, cleanup)
		assert.Equal(t, filepath.Join(componentDir, "external.plan"), result)

		// Verify the file was copied
		copiedData, err := os.ReadFile(result)
		assert.NoError(t, err)
		assert.Equal(t, "external plan data", string(copiedData))

		// Call cleanup and verify file is removed
		cleanup()
		_, err = os.Stat(result)
		assert.True(t, os.IsNotExist(err))
	})

	// Test 3: Non-existent source file
	t.Run("non_existent_source_file", func(t *testing.T) {
		nonExistent := filepath.Join(tmpDir, "non-existent.plan")
		_, _, err := copyPlanFileIfNeeded(nonExistent, componentDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error opening source plan file")
	})
}

// TestGetResourceAttributes tests the getResourceAttributes function.
func TestGetResourceAttributes(t *testing.T) {
	// Test with values field
	t.Run("with_values_field", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"values": map[string]interface{}{
				"id":   "i-12345",
				"name": "example",
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-12345", attrs["id"])
		assert.Equal(t, "example", attrs["name"])
	})

	// Test with change.after field
	t.Run("with_change_after_field", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"id":   "i-67890",
					"name": "changed",
				},
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-67890", attrs["id"])
		assert.Equal(t, "changed", attrs["name"])
	})

	// Test with both fields (values should be overwritten by change.after)
	t.Run("with_both_fields", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"values": map[string]interface{}{
				"id":   "i-12345",
				"name": "original",
			},
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"id":   "i-67890",
					"name": "changed",
				},
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-67890", attrs["id"])
		assert.Equal(t, "changed", attrs["name"])
	})

	// Test with non-map resource
	t.Run("non_map_resource", func(t *testing.T) {
		attrs := getResourceAttributes("not a map")
		assert.Empty(t, attrs)
	})

	// Test with empty resource
	t.Run("empty_resource", func(t *testing.T) {
		attrs := getResourceAttributes(map[string]interface{}{})
		assert.Empty(t, attrs)
	})
}

// TestExtractValuesField tests the extractValuesField function.
func TestExtractValuesField(t *testing.T) {
	t.Run("with_values", func(t *testing.T) {
		resMap := map[string]interface{}{
			"values": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("without_values", func(t *testing.T) {
		resMap := map[string]interface{}{
			"other": "data",
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("values_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"values": "not a map",
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Empty(t, result)
	})
}

// TestExtractChangeAfterField tests the extractChangeAfterField function.
func TestExtractChangeAfterField(t *testing.T) {
	t.Run("with_change_after", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("without_change", func(t *testing.T) {
		resMap := map[string]interface{}{
			"other": "data",
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("change_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": "not a map",
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("after_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": map[string]interface{}{
				"after": "not a map",
			},
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})
}

// TestProcessRootModuleResources tests the processRootModuleResources function.
func TestProcessRootModuleResources(t *testing.T) {
	t.Run("with_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"type":    "aws_instance",
					"name":    "example",
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.data",
					"mode":    "managed",
					"type":    "aws_s3_bucket",
					"name":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Len(t, result, 2)
		assert.NotNil(t, result["aws_instance.example"])
		assert.NotNil(t, result["aws_s3_bucket.data"])
	})

	t.Run("skip_data_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
				},
				map[string]interface{}{
					"address": "data.aws_ami.ubuntu",
					"mode":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
		assert.Nil(t, result["data.aws_ami.ubuntu"])
	})

	t.Run("no_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Empty(t, result)
	})

	t.Run("invalid_resource_format", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				"not a map",
				map[string]interface{}{
					// missing address
					"mode": "managed",
				},
				map[string]interface{}{
					"address": "aws_instance.example",
					// missing mode
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Empty(t, result)
	})
}

// TestProcessResourceChanges tests the processResourceChanges function.
func TestProcessResourceChanges(t *testing.T) {
	t.Run("with_resource_changes", func(t *testing.T) {
		plan := map[string]interface{}{
			"resource_changes": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []interface{}{"update"},
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.data",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []interface{}{"create"},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Len(t, result, 2)
		assert.NotNil(t, result["aws_instance.example"])
		assert.NotNil(t, result["aws_s3_bucket.data"])
	})

	t.Run("skip_data_resources", func(t *testing.T) {
		plan := map[string]interface{}{
			"resource_changes": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
				},
				map[string]interface{}{
					"address": "data.aws_ami.ubuntu",
					"mode":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_resource_changes", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Empty(t, result)
	})
}

// TestProcessPriorStateResources tests the processPriorStateResources function.
func TestProcessPriorStateResources(t *testing.T) {
	t.Run("with_prior_state", func(t *testing.T) {
		plan := map[string]interface{}{
			"prior_state": map[string]interface{}{
				"values": map[string]interface{}{
					"root_module": map[string]interface{}{
						"resources": []interface{}{
							map[string]interface{}{
								"address": "aws_instance.example",
								"mode":    "managed",
							},
						},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_prior_state", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Empty(t, result)
	})

	t.Run("invalid_prior_state_structure", func(t *testing.T) {
		plan := map[string]interface{}{
			"prior_state": "not a map",
		}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Empty(t, result)
	})
}

// TestProcessPlannedValuesResources tests the processPlannedValuesResources function.
func TestProcessPlannedValuesResources(t *testing.T) {
	t.Run("with_planned_values", func(t *testing.T) {
		plan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.example",
							"mode":    "managed",
						},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processPlannedValuesResources(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_planned_values", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processPlannedValuesResources(plan, result)
		assert.Empty(t, result)
	})
}

// TestCompareResources tests the compareResources function.
func TestCompareResources(t *testing.T) {
	t.Run("added_resources", func(t *testing.T) {
		origResources := map[string]interface{}{}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "+ aws_instance.example")
	})

	t.Run("removed_resources", func(t *testing.T) {
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		newResources := map[string]interface{}{}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "- aws_instance.example")
	})

	t.Run("changed_resources", func(t *testing.T) {
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
				"values": map[string]interface{}{
					"instance_type": "t2.micro",
				},
			},
		}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
				"values": map[string]interface{}{
					"instance_type": "t2.large",
				},
			},
		}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "aws_instance.example")
		assert.Contains(t, result, "instance_type")
	})

	t.Run("no_changes", func(t *testing.T) {
		resources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		result := compareResources(resources, resources)
		assert.Empty(t, result)
	})
}

// TestProcessResourceAdditionsAndRemovals tests the processResourceAdditionsAndRemovals function.
func TestProcessResourceAdditionsAndRemovals(t *testing.T) {
	t.Run("additions_and_removals", func(t *testing.T) {
		var diff strings.Builder
		origResources := map[string]interface{}{
			"aws_instance.old": map[string]interface{}{},
		}
		newResources := map[string]interface{}{
			"aws_instance.new": map[string]interface{}{},
		}
		processResourceAdditionsAndRemovals(&diff, origResources, newResources)
		result := diff.String()
		assert.Contains(t, result, "+ aws_instance.new")
		assert.Contains(t, result, "- aws_instance.old")
	})
}

// TestProcessChangedResources tests the processChangedResources function.
func TestProcessChangedResources(t *testing.T) {
	t.Run("with_attribute_changes", func(t *testing.T) {
		var diff strings.Builder
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"values": map[string]interface{}{
					"instance_type": "t2.micro",
					"ami":           "ami-12345",
				},
			},
		}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"values": map[string]interface{}{
					"instance_type": "t2.large",
					"ami":           "ami-12345",
				},
			},
		}
		processChangedResources(&diff, origResources, newResources)
		result := diff.String()
		assert.Contains(t, result, "aws_instance.example")
		assert.Contains(t, result, "instance_type")
	})
}

// TestProcessAttributeDifferences tests the processAttributeDifferences function.
func TestProcessAttributeDifferences(t *testing.T) {
	t.Run("priority_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"id":  "old-id",
			"url": "http://old.com",
		}
		newAttrs := map[string]interface{}{
			"id":  "new-id",
			"url": "http://new.com",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		// Priority attributes (id, url) should be shown
		assert.Contains(t, result, "id")
		assert.Contains(t, result, "url")
	})

	t.Run("skip_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"content":              "old content",
			"content_md5":          "old-hash",
			"content_sha256":       "old-sha",
			"response_body_base64": "old-body",
		}
		newAttrs := map[string]interface{}{
			"content":              "new content",
			"content_md5":          "new-hash",
			"content_sha256":       "new-sha",
			"response_body_base64": "new-body",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		// content is a priority attribute, so it should be shown
		assert.Contains(t, result, "content")
		// Skip attributes should not be shown
		assert.NotContains(t, result, "content_md5")
		assert.NotContains(t, result, "content_sha256")
		assert.NotContains(t, result, "response_body_base64")
	})

	t.Run("added_and_removed_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"old_attr": "old_value",
		}
		newAttrs := map[string]interface{}{
			"new_attr": "new_value",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		assert.Contains(t, result, "- old_attr")
		assert.Contains(t, result, "+ new_attr")
	})
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

// TestCompareVariables tests the compareVariables function.
func TestCompareVariables(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, origPlan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("added_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "+ location")
	})

	t.Run("removed_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "- location")
	})

	t.Run("changed_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "New York"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "~ location")
	})
}

// TestCompareOutputSections tests the compareOutputSections function.
func TestCompareOutputSections(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
		plan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://example.com"},
				},
			},
		}
		diff, hasDiff := compareOutputSections(plan, plan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("with_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://old.com"},
				},
			},
		}
		newPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://new.com"},
				},
			},
		}
		diff, hasDiff := compareOutputSections(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "Outputs:")
	})
}

// TestCompareResourceSections tests the compareResourceSections function.
func TestCompareResourceSections(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
		plan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.example",
							"mode":    "managed",
						},
					},
				},
			},
		}
		diff, hasDiff := compareResourceSections(plan, plan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("with_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.old",
							"mode":    "managed",
						},
					},
				},
			},
		}
		newPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.new",
							"mode":    "managed",
						},
					},
				},
			},
		}
		diff, hasDiff := compareResourceSections(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "Resources:")
	})
}

// TestGetResources tests the getResources function.
func TestGetResources(t *testing.T) {
	plan := map[string]interface{}{
		"prior_state": map[string]interface{}{
			"values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.prior",
							"mode":    "managed",
						},
					},
				},
			},
		},
		"planned_values": map[string]interface{}{
			"root_module": map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"address": "aws_instance.planned",
						"mode":    "managed",
					},
				},
			},
		},
		"resource_changes": []interface{}{
			map[string]interface{}{
				"address": "aws_instance.changed",
				"mode":    "managed",
			},
		},
	}

	resources := getResources(plan)
	assert.NotNil(t, resources["aws_instance.prior"])
	assert.NotNil(t, resources["aws_instance.planned"])
	assert.NotNil(t, resources["aws_instance.changed"])
}

// TestCompareOutputs tests the compareOutputs function.
func TestCompareOutputs(t *testing.T) {
	t.Run("added_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{}
		newOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://example.com"},
		}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "+ url")
	})

	t.Run("removed_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://example.com"},
		}
		newOutputs := map[string]interface{}{}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "- url")
	})

	t.Run("changed_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://old.com"},
		}
		newOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://new.com"},
		}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "~ url")
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

// TestTerraformPlanDiffWithWorkdir tests that plan-diff correctly handles workdir scenarios.
// When workdir is enabled, the component path should be the workdir path, not the base component path.
// This test verifies the path resolution logic works with workdir provisioning.
func TestTerraformPlanDiffWithWorkdir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the base component directory structure
	baseComponentDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	err := os.MkdirAll(baseComponentDir, 0o755)
	require.NoError(t, err)

	// Create the workdir directory (what workdir provisioner creates)
	workdirPath := filepath.Join(tmpDir, ".workdir", "terraform", "dev-vpc")
	err = os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create planfile in the WORKDIR (where it would be after workdir provisioning)
	planfileName := "test.planfile"
	planfileInWorkdir := filepath.Join(workdirPath, planfileName)
	err = os.WriteFile(planfileInWorkdir, []byte("dummy plan data from workdir"), 0o644)
	require.NoError(t, err)

	terraformDirPath := filepath.Join(tmpDir, "components", "terraform")

	// Test 1: Planfile is found in workdir path (correct behavior)
	t.Run("planfile_found_in_workdir", func(t *testing.T) {
		// When workdir is enabled, the componentPath should be the workdir path
		componentPath := workdirPath

		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfileInWorkdir, result)
	})

	// Test 2: Planfile NOT found in base component path (demonstrates the issue)
	t.Run("planfile_not_in_base_component_path", func(t *testing.T) {
		// If we incorrectly use the base component path instead of workdir,
		// the planfile won't be found
		componentPath := filepath.Join(terraformDirPath, "vpc")

		_, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	// Test 3: Verify WorkdirPathKey extraction from ComponentSection
	t.Run("workdir_path_key_extraction", func(t *testing.T) {
		// This tests the pattern used in terraform.go and other commands
		// to extract the workdir path from ComponentSection
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		}

		// Extract workdir path as done in terraform.go
		if extractedWorkdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string); ok {
			assert.Equal(t, workdirPath, extractedWorkdir)

			// Validate planfile is found at workdir path
			result, err := validateOriginalPlanFile(planfileName, extractedWorkdir)
			assert.NoError(t, err)
			assert.Equal(t, planfileInWorkdir, result)
		} else {
			t.Fatal("Failed to extract workdir path from ComponentSection")
		}
	})

	// Test 4: Empty workdir path should fall back to base component path
	t.Run("empty_workdir_falls_back_to_base", func(t *testing.T) {
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: "",
		}

		// When workdir path is empty, should use base component path
		extractedWorkdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string)
		if ok && extractedWorkdir != "" {
			t.Fatal("Expected empty workdir path")
		}

		// With empty workdir, would fall back to base component path
		// Create planfile in base component for this test
		planfileInBase := filepath.Join(baseComponentDir, planfileName)
		err := os.WriteFile(planfileInBase, []byte("dummy plan data from base"), 0o644)
		require.NoError(t, err)
		defer os.Remove(planfileInBase)

		result, err := validateOriginalPlanFile(planfileName, baseComponentDir)
		assert.NoError(t, err)
		assert.Equal(t, planfileInBase, result)
	})
}

// TestTerraformPlanDiffWithSourceVendoring tests that plan-diff correctly handles
// source-vendored components with workdir enabled.
// When source + workdir are both enabled, the planfile should be in the workdir path.
func TestTerraformPlanDiffWithSourceVendoring(t *testing.T) {
	tmpDir := t.TempDir()

	// Source + workdir scenario:
	// 1. Component is vendored from remote source
	// 2. Workdir is enabled, so files go to .workdir/terraform/<stack>-<component>/
	// 3. Planfile should be looked up in the workdir path

	// Create the workdir path where source provisioner would vendor files
	workdirPath := filepath.Join(tmpDir, ".workdir", "terraform", "plat-ue2-sandbox-foobar-atmos-pro")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create a mock terraform file to simulate vendored component
	mainTf := filepath.Join(workdirPath, "main.tf")
	err = os.WriteFile(mainTf, []byte(`resource "null_resource" "example" {}`), 0o644)
	require.NoError(t, err)

	// Create the planfile in the workdir (where it would be stored after terraform plan)
	planfileName := "plat-ue2-sandbox-foobar-atmos-pro.planfile"
	planfilePath := filepath.Join(workdirPath, planfileName)
	err = os.WriteFile(planfilePath, []byte("dummy planfile for source+workdir test"), 0o644)
	require.NoError(t, err)

	// Simulate what would happen in TerraformPlanDiff with source + workdir
	t.Run("source_vendored_with_workdir", func(t *testing.T) {
		// After ProcessStacks, the component section would have WorkdirPathKey set
		// by the source provisioner when source + workdir are enabled
		componentSection := map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		}

		// Extract workdir path as done in terraform.go
		componentPath := ""
		if workdir, ok := componentSection[provWorkdir.WorkdirPathKey].(string); ok && workdir != "" {
			componentPath = workdir
		}

		assert.Equal(t, workdirPath, componentPath)

		// Validate planfile is found
		result, err := validateOriginalPlanFile(planfileName, componentPath)
		assert.NoError(t, err)
		assert.Equal(t, planfilePath, result)
	})

	// Test the error case when workdir path is not used
	t.Run("source_vendored_without_workdir_path_fails", func(t *testing.T) {
		// If plan-diff doesn't check WorkdirPathKey and uses base component path,
		// it won't find the planfile
		baseComponentPath := filepath.Join(tmpDir, "components", "terraform", "foobar")

		_, err := validateOriginalPlanFile(planfileName, baseComponentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})
}
