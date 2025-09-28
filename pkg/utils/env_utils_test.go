package utils

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
)

func TestConvertEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name:     "nil map",
			input:    nil,
			expected: []string{},
		},
		{
			name: "single string value",
			input: map[string]any{
				"KEY1": "value1",
			},
			expected: []string{"KEY1=value1"},
		},
		{
			name: "multiple string values",
			input: map[string]any{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
			expected: []string{"KEY1=value1", "KEY2=value2", "KEY3=value3"},
		},
		{
			name: "mixed types",
			input: map[string]any{
				"STRING_VAR": "string_value",
				"INT_VAR":    123,
				"BOOL_VAR":   true,
				"FLOAT_VAR":  3.14,
			},
			expected: []string{
				"STRING_VAR=string_value",
				"INT_VAR=123",
				"BOOL_VAR=true",
				"FLOAT_VAR=3.14",
			},
		},
		{
			name: "null string value should be excluded",
			input: map[string]any{
				"VALID_KEY":   "valid_value",
				"NULL_KEY":    "null",
				"ANOTHER_KEY": "another_value",
			},
			expected: []string{"VALID_KEY=valid_value", "ANOTHER_KEY=another_value"},
		},
		{
			name: "nil value should be excluded",
			input: map[string]any{
				"VALID_KEY": "valid_value",
				"NIL_KEY":   nil,
			},
			expected: []string{"VALID_KEY=valid_value"},
		},
		{
			name: "empty string value should be included",
			input: map[string]any{
				"EMPTY_KEY": "",
				"VALID_KEY": "value",
			},
			expected: []string{"EMPTY_KEY=", "VALID_KEY=value"},
		},
		{
			name: "zero values should be included",
			input: map[string]any{
				"ZERO_INT":   0,
				"FALSE_VAL":  false,
				"ZERO_FLOAT": 0.0,
			},
			expected: []string{"ZERO_INT=0", "FALSE_VAL=false", "ZERO_FLOAT=0"},
		},
		{
			name: "special characters in values",
			input: map[string]any{
				"SPECIAL_CHARS": "value with spaces and symbols!@#$%",
				"EQUALS_SIGN":   "key=value=more",
				"QUOTES":        `"quoted value"`,
			},
			expected: []string{
				"SPECIAL_CHARS=value with spaces and symbols!@#$%",
				"EQUALS_SIGN=key=value=more",
				"QUOTES=\"quoted value\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertEnvVars(tt.input)

			// Sort both slices to ensure consistent comparison since map iteration order is not guaranteed.
			sort.Strings(result)
			sort.Strings(tt.expected)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ConvertEnvVars() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestConvertEnvVars_DeterministicOrder(t *testing.T) {
	// Test that multiple calls with the same input produce results with the same elements
	// (order may vary due to map iteration, but content should be identical).
	input := map[string]any{
		"KEY1": "value1",
		"KEY2": "value2",
		"KEY3": "value3",
	}

	result1 := ConvertEnvVars(input)
	result2 := ConvertEnvVars(input)

	// Sort both results and compare
	sort.Strings(result1)
	sort.Strings(result2)

	if !reflect.DeepEqual(result1, result2) {
		t.Errorf("ConvertEnvVars() should produce consistent results, got %v and %v", result1, result2)
	}
}

func TestEnvironToMap(t *testing.T) {
	tests := []struct {
		name        string
		setupEnvs   map[string]string // Environment variables to set for the test
		cleanupEnvs []string          // Environment variables to unset after test
	}{
		{
			name: "basic environment variables",
			setupEnvs: map[string]string{
				"TEST_VAR_1": "value1",
				"TEST_VAR_2": "value2",
			},
			cleanupEnvs: []string{"TEST_VAR_1", "TEST_VAR_2"},
		},
		{
			name: "environment variable with empty value",
			setupEnvs: map[string]string{
				"EMPTY_VAR": "",
			},
			cleanupEnvs: []string{"EMPTY_VAR"},
		},
		{
			name: "environment variable with special characters",
			setupEnvs: map[string]string{
				"SPECIAL_VAR": "value with spaces and symbols!@#$%^&*()",
			},
			cleanupEnvs: []string{"SPECIAL_VAR"},
		},
		{
			name: "environment variable with equals signs in value",
			setupEnvs: map[string]string{
				"EQUALS_VAR": "key=value=more=data",
			},
			cleanupEnvs: []string{"EQUALS_VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: Set test environment variables.
			originalEnvs := make(map[string]string)
			for key, value := range tt.setupEnvs {
				if originalVal, exists := os.LookupEnv(key); exists {
					originalEnvs[key] = originalVal
				}
				os.Setenv(key, value)
			}

			// Cleanup: Restore original environment after test.
			defer func() {
				for _, key := range tt.cleanupEnvs {
					if originalVal, exists := originalEnvs[key]; exists {
						os.Setenv(key, originalVal)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Test the function
			result := EnvironToMap()

			// Verify that our test environment variables are present with correct values.
			for key, expectedValue := range tt.setupEnvs {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected environment variable %s to be present in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("Expected %s=%s, got %s=%s", key, expectedValue, key, actualValue)
				}
			}
		})
	}
}

func TestEnvironToMap_SystemEnvironment(t *testing.T) {
	// Test that the function returns a map that includes actual system environment variables.
	result := EnvironToMap()

	// Get the current environment using os.Environ().
	systemEnv := os.Environ()

	// Verify that the number of entries matches
	if len(result) != len(systemEnv) {
		t.Errorf("Expected %d environment variables, got %d", len(systemEnv), len(result))
	}

	// Verify that each system environment variable is correctly parsed.
	for _, envPair := range systemEnv {
		// Split at first occurrence of '=' (same logic as EnvironToMap)
		parts := SplitStringAtFirstOccurrence(envPair, "=")
		if len(parts) != 2 {
			t.Errorf("Invalid environment variable format: %s", envPair)
			continue
		}

		key := parts[0]
		expectedValue := parts[1]

		if actualValue, exists := result[key]; !exists {
			t.Errorf("Environment variable %s not found in result", key)
		} else if actualValue != expectedValue {
			t.Errorf("Mismatch for %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}
}

func TestEnvironToMap_EmptyValues(t *testing.T) {
	// Test handling of environment variables with empty values.
	testKey := "TEST_EMPTY_VALUE"

	// Ensure the test key doesn't exist initially
	originalVal, existed := os.LookupEnv(testKey)

	// Set an empty value
	os.Setenv(testKey, "")

	// Cleanup after test
	defer func() {
		if existed {
			os.Setenv(testKey, originalVal)
		} else {
			os.Unsetenv(testKey)
		}
	}()

	result := EnvironToMap()

	if value, exists := result[testKey]; !exists {
		t.Errorf("Expected environment variable %s with empty value to be present", testKey)
	} else if value != "" {
		t.Errorf("Expected empty value for %s, got %q", testKey, value)
	}
}

func TestEnvironToMap_SpecialCharactersInValue(t *testing.T) {
	// Test handling of environment variables with special characters in values.
	testCases := []struct {
		key   string
		value string
	}{
		{"TEST_SPACES", "value with spaces"},
		{"TEST_SYMBOLS", "!@#$%^&*()"},
		{"TEST_QUOTES", `"quoted value"`},
		{"TEST_NEWLINES", "line1\nline2"},
		{"TEST_TABS", "value\twith\ttabs"},
		{"TEST_UNICODE", "value with unicode: 🚀 ñ ü"},
	}

	// Store original values for cleanup
	originalVals := make(map[string]string)
	existedVals := make(map[string]bool)

	for _, tc := range testCases {
		if val, existed := os.LookupEnv(tc.key); existed {
			originalVals[tc.key] = val
			existedVals[tc.key] = true
		}
		os.Setenv(tc.key, tc.value)
	}

	// Cleanup after test
	defer func() {
		for _, tc := range testCases {
			if existedVals[tc.key] {
				os.Setenv(tc.key, originalVals[tc.key])
			} else {
				os.Unsetenv(tc.key)
			}
		}
	}()

	result := EnvironToMap()

	for _, tc := range testCases {
		if value, exists := result[tc.key]; !exists {
			t.Errorf("Expected environment variable %s to be present", tc.key)
		} else if value != tc.value {
			t.Errorf("Mismatch for %s: expected %q, got %q", tc.key, tc.value, value)
		}
	}
}

func TestEnvironToMap_KeysWithEqualsInValue(t *testing.T) {
	// Test the specific case where environment variable values contain equals signs
	// This tests the SplitStringAtFirstOccurrence logic
	testKey := "TEST_EQUALS_IN_VALUE"
	testValue := "key=value=more=data"

	originalVal, existed := os.LookupEnv(testKey)

	os.Setenv(testKey, testValue)

	defer func() {
		if existed {
			os.Setenv(testKey, originalVal)
		} else {
			os.Unsetenv(testKey)
		}
	}()

	result := EnvironToMap()

	if value, exists := result[testKey]; !exists {
		t.Errorf("Expected environment variable %s to be present", testKey)
	} else if value != testValue {
		t.Errorf("Expected %s=%s, got %s=%s", testKey, testValue, testKey, value)
	}
}

func TestEnvironToMap_ErrorHandling(t *testing.T) {
	// Test that the function handles edge cases gracefully.
	// Note: We can't easily create malformed environment variables through os.Setenv,
	// but we can test that the function doesn't panic and handles normal cases properly.

	testKey := "TEST_ERROR_HANDLING"
	testValue := ""

	originalVal, existed := os.LookupEnv(testKey)

	os.Setenv(testKey, testValue)

	defer func() {
		if existed {
			os.Setenv(testKey, originalVal)
		} else {
			os.Unsetenv(testKey)
		}
	}()

	// Should not panic and should handle empty values correctly.
	result := EnvironToMap()

	if value, exists := result[testKey]; !exists {
		t.Errorf("Expected environment variable %s with empty value to be present", testKey)
	} else if value != testValue {
		t.Errorf("Expected %s=%s, got %s=%s", testKey, testValue, testKey, value)
	}
}

// Benchmark tests to ensure functions perform well with larger inputs.
func BenchmarkConvertEnvVars(b *testing.B) {
	// Create a large map for benchmarking.
	largeMap := make(map[string]any)
	for i := 0; i < 1000; i++ {
		largeMap[fmt.Sprintf("KEY_%d", i)] = fmt.Sprintf("value_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConvertEnvVars(largeMap)
	}
}

func BenchmarkEnvironToMap(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EnvironToMap()
	}
}
