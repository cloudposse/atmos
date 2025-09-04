package utils

import (
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertEnvVars(t *testing.T) {
	tests := []struct {
		name                string
		envVarsMap          map[string]any
		expectedContains    []string
		expectedNotContains []string
		expectedLen         int
	}{
		{
			name: "simple string values",
			envVarsMap: map[string]any{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
			expectedContains: []string{
				"KEY1=value1",
				"KEY2=value2",
				"KEY3=value3",
			},
			expectedLen: 3,
		},
		{
			name: "mixed types",
			envVarsMap: map[string]any{
				"STRING": "hello",
				"INT":    42,
				"FLOAT":  3.14,
				"BOOL":   true,
			},
			expectedContains: []string{
				"STRING=hello",
				"INT=%!s(int=42)",         // fmt.Sprintf with %s on int
				"FLOAT=%!s(float64=3.14)", // fmt.Sprintf with %s on float
				"BOOL=%!s(bool=true)",     // fmt.Sprintf with %s on bool
			},
			expectedLen: 4,
		},
		{
			name: "with nil values",
			envVarsMap: map[string]any{
				"VALID":  "value",
				"NIL":    nil,
				"VALID2": "value2",
			},
			expectedContains: []string{
				"VALID=value",
				"VALID2=value2",
			},
			expectedNotContains: []string{
				"NIL=",
			},
			expectedLen: 2, // nil values are skipped
		},
		{
			name: "with null string values",
			envVarsMap: map[string]any{
				"VALID":       "value",
				"NULL_STRING": "null",
				"VALID2":      "value2",
			},
			expectedContains: []string{
				"VALID=value",
				"VALID2=value2",
			},
			expectedNotContains: []string{
				"NULL_STRING=null",
			},
			expectedLen: 2, // "null" strings are skipped
		},
		{
			name:        "empty map",
			envVarsMap:  map[string]any{},
			expectedLen: 0,
		},
		{
			name:        "nil map",
			envVarsMap:  nil,
			expectedLen: 0,
		},
		{
			name: "values with special characters",
			envVarsMap: map[string]any{
				"PATH":   "/usr/bin:/usr/local/bin",
				"SPACES": "hello world",
				"EQUALS": "key=value",
				"QUOTES": `"quoted"`,
			},
			expectedContains: []string{
				"PATH=/usr/bin:/usr/local/bin",
				"SPACES=hello world",
				"EQUALS=key=value",
				`QUOTES="quoted"`,
			},
			expectedLen: 4,
		},
		{
			name: "empty string values",
			envVarsMap: map[string]any{
				"EMPTY": "",
				"VALID": "value",
			},
			expectedContains: []string{
				"EMPTY=",
				"VALID=value",
			},
			expectedLen: 2,
		},
		{
			name: "only nil and null values",
			envVarsMap: map[string]any{
				"NIL1": nil,
				"NIL2": nil,
				"NULL": "null",
			},
			expectedLen: 0, // All should be skipped
		},
		{
			name: "nested structures",
			envVarsMap: map[string]any{
				"SLICE": []int{1, 2, 3},
				"MAP":   map[string]int{"nested": 1},
			},
			expectedContains: []string{
				"SLICE=[%!s(int=1) %!s(int=2) %!s(int=3)]",
				"MAP=map[nested:%!s(int=1)]",
			},
			expectedLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertEnvVars(tt.envVarsMap)

			// Check length
			assert.Len(t, result, tt.expectedLen)

			// Check expected values are present
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}

			// Check values that should not be present
			for _, notExpected := range tt.expectedNotContains {
				assert.NotContains(t, result, notExpected)
			}
		})
	}
}

func TestEnvironToMap(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		// Restore original environment
		os.Clearenv()
		for _, env := range origEnv {
			pair := SplitStringAtFirstOccurrence(env, "=")
			os.Setenv(pair[0], pair[1])
		}
	}()

	tests := []struct {
		name   string
		setup  func()
		verify func(t *testing.T, envMap map[string]string)
	}{
		{
			name: "basic environment variables",
			setup: func() {
				os.Clearenv()
				os.Setenv("TEST_KEY1", "value1")
				os.Setenv("TEST_KEY2", "value2")
				os.Setenv("TEST_KEY3", "value3")
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Equal(t, "value1", envMap["TEST_KEY1"])
				assert.Equal(t, "value2", envMap["TEST_KEY2"])
				assert.Equal(t, "value3", envMap["TEST_KEY3"])
				assert.Len(t, envMap, 3)
			},
		},
		{
			name: "empty environment",
			setup: func() {
				os.Clearenv()
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Empty(t, envMap)
			},
		},
		{
			name: "values with equals signs",
			setup: func() {
				os.Clearenv()
				os.Setenv("KEY_WITH_EQUALS", "value=with=equals")
				os.Setenv("ANOTHER_KEY", "normal_value")
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Equal(t, "value=with=equals", envMap["KEY_WITH_EQUALS"])
				assert.Equal(t, "normal_value", envMap["ANOTHER_KEY"])
				assert.Len(t, envMap, 2)
			},
		},
		{
			name: "empty values",
			setup: func() {
				os.Clearenv()
				os.Setenv("EMPTY_VALUE", "")
				os.Setenv("NON_EMPTY", "value")
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Equal(t, "", envMap["EMPTY_VALUE"])
				assert.Equal(t, "value", envMap["NON_EMPTY"])
				assert.Len(t, envMap, 2)
			},
		},
		{
			name: "special characters in values",
			setup: func() {
				os.Clearenv()
				os.Setenv("PATH_VAR", "/usr/bin:/usr/local/bin")
				os.Setenv("SPACES", "hello world")
				os.Setenv("TABS", "tab\ttab")
				os.Setenv("NEWLINES", "line1\nline2")
				os.Setenv("QUOTES", `"quoted"`)
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Equal(t, "/usr/bin:/usr/local/bin", envMap["PATH_VAR"])
				assert.Equal(t, "hello world", envMap["SPACES"])
				assert.Equal(t, "tab\ttab", envMap["TABS"])
				assert.Equal(t, "line1\nline2", envMap["NEWLINES"])
				assert.Equal(t, `"quoted"`, envMap["QUOTES"])
				assert.Len(t, envMap, 5)
			},
		},
		{
			name: "unicode values",
			setup: func() {
				os.Clearenv()
				os.Setenv("UNICODE1", "こんにちは")
				os.Setenv("UNICODE2", "你好")
				os.Setenv("UNICODE3", "مرحبا")
			},
			verify: func(t *testing.T, envMap map[string]string) {
				assert.Equal(t, "こんにちは", envMap["UNICODE1"])
				assert.Equal(t, "你好", envMap["UNICODE2"])
				assert.Equal(t, "مرحبا", envMap["UNICODE3"])
				assert.Len(t, envMap, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()
			envMap := EnvironToMap()
			tt.verify(t, envMap)
		})
	}
}

func TestConvertEnvVarsStability(t *testing.T) {
	// Test that the function produces consistent results
	envVarsMap := map[string]any{
		"KEY1": "value1",
		"KEY2": "value2",
		"KEY3": "value3",
	}

	// Run multiple times and verify results are equivalent
	results := [][]string{}
	for i := 0; i < 5; i++ {
		result := ConvertEnvVars(envVarsMap)
		sort.Strings(result) // Sort for comparison
		results = append(results, result)
	}

	// All results should be the same when sorted
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i], "Results should be consistent across multiple calls")
	}
}

func TestEnvironToMapWithRealEnvironment(t *testing.T) {
	// Test with the real environment (don't clear it)
	envMap := EnvironToMap()

	// Verify that at least some standard environment variables are present
	// (these might not exist on all systems, so we check if they exist first)
	possibleVars := []string{"PATH", "HOME", "USER", "SHELL", "PWD"}

	foundAtLeastOne := false
	for _, varName := range possibleVars {
		if osValue, exists := os.LookupEnv(varName); exists {
			foundAtLeastOne = true
			mapValue, mapExists := envMap[varName]
			assert.True(t, mapExists, "Environment variable %s should be in map", varName)
			assert.Equal(t, osValue, mapValue, "Value for %s should match", varName)
		}
	}

	// On most systems, at least one of these variables should exist
	assert.True(t, foundAtLeastOne, "Should find at least one standard environment variable")

	// The map should not be empty on most systems
	assert.NotEmpty(t, envMap, "Environment map should not be empty on most systems")
}

func TestConvertEnvVarsOrder(t *testing.T) {
	// Test that nil and "null" values are consistently filtered
	envVarsMap := map[string]any{
		"A": "valueA",
		"B": nil,
		"C": "null",
		"D": "valueD",
		"E": nil,
		"F": "valueF",
	}

	result := ConvertEnvVars(envVarsMap)

	// Should only have A, D, F
	assert.Len(t, result, 3)

	// Sort for consistent checking
	sort.Strings(result)
	expected := []string{"A=valueA", "D=valueD", "F=valueF"}
	sort.Strings(expected)

	assert.Equal(t, expected, result)
}
