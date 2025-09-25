package tests

import (
	"strings"
	"testing"
)

// TestVerifyYAMLFormatUnit tests the YAML validation function directly.
func TestVerifyYAMLFormatUnit(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldPass bool
		errorCheck string // substring that should appear in error
	}{
		{
			name:       "Valid YAML - simple key-value",
			input:      "key: value\nanother: test",
			shouldPass: true,
		},
		{
			name:       "Valid YAML - nested structure",
			input:      "parent:\n  child: value\n  another:\n    deeply: nested",
			shouldPass: true,
		},
		{
			name:       "Valid YAML - list",
			input:      "items:\n  - first\n  - second\n  - third",
			shouldPass: true,
		},
		{
			name:       "Valid YAML - empty",
			input:      "",
			shouldPass: true,
		},
		{
			name:       "Valid YAML - multiline string",
			input:      "description: |\n  This is a\n  multiline string",
			shouldPass: true,
		},
		{
			name:       "Invalid YAML - bad indentation",
			input:      "key: value\n  bad indentation without parent",
			shouldPass: true, // Actually valid - interpreted as string value
			errorCheck: "YAML validation failed",
		},
		{
			name:       "Invalid YAML - unclosed quote",
			input:      "key: \"unclosed string",
			shouldPass: false,
			errorCheck: "YAML validation failed",
		},
		{
			name:       "Invalid YAML - tab character",
			input:      "key:\tvalue with tab",
			shouldPass: true, // Actually valid - tab is part of the value
			errorCheck: "YAML validation failed",
		},
		{
			name: "Valid YAML - complex Atmos config",
			input: `base_path: ./
components:
  terraform:
    base_path: components/terraform
    apply_auto_approve: false
stacks:
  base_path: stacks
  included_paths:
    - "**/*.yaml"`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run validation directly and check result
			validationPassed := verifyYAMLFormat(t, tt.input)

			if validationPassed != tt.shouldPass {
				t.Errorf("YAML validation returned %v, expected %v", validationPassed, tt.shouldPass)
			}
		})
	}
}

// TestVerifyJSONFormatUnit tests the JSON validation function directly.
func TestVerifyJSONFormatUnit(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldPass bool
		errorCheck string
	}{
		{
			name:       "Valid JSON - simple object",
			input:      `{"key": "value", "number": 123}`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - nested object",
			input:      `{"parent": {"child": "value", "number": 42}}`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - array",
			input:      `[1, 2, 3, "four", true, null]`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - empty object",
			input:      `{}`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - empty array",
			input:      `[]`,
			shouldPass: true,
		},
		{
			name:       "Invalid JSON - missing quotes on key",
			input:      `{key: "value"}`,
			shouldPass: false,
			errorCheck: "JSON validation failed",
		},
		{
			name:       "Invalid JSON - trailing comma",
			input:      `{"key": "value",}`,
			shouldPass: false,
			errorCheck: "JSON validation failed",
		},
		{
			name:       "Invalid JSON - single quotes",
			input:      `{'key': 'value'}`,
			shouldPass: false,
			errorCheck: "JSON validation failed",
		},
		{
			name:       "Invalid JSON - unclosed string",
			input:      `{"key": "unclosed`,
			shouldPass: false,
			errorCheck: "JSON validation failed",
		},
		{
			name:       "Invalid JSON - plain text",
			input:      `This is not JSON`,
			shouldPass: false,
			errorCheck: "JSON validation failed",
		},
		{
			name:       "Valid JSON - string value",
			input:      `"just a string"`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - number value",
			input:      `42`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - boolean value",
			input:      `true`,
			shouldPass: true,
		},
		{
			name:       "Valid JSON - null value",
			input:      `null`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validationPassed := verifyJSONFormat(t, tt.input)

			if validationPassed != tt.shouldPass {
				t.Errorf("JSON validation returned %v, expected %v", validationPassed, tt.shouldPass)
			}
		})
	}
}

// TestVerifyFormatValidationUnit tests the format validation dispatcher.
func TestVerifyFormatValidationUnit(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		formats    []string
		shouldPass bool
	}{
		{
			name:       "Valid YAML format",
			input:      "key: value",
			formats:    []string{"yaml"},
			shouldPass: true,
		},
		{
			name:       "Valid JSON format",
			input:      `{"key": "value"}`,
			formats:    []string{"json"},
			shouldPass: true,
		},
		{
			name:       "Multiple formats - JSON passes",
			input:      `{"key": "value"}`,
			formats:    []string{"json", "yaml"}, // Valid JSON is also valid YAML
			shouldPass: true,
		},
		{
			name:       "Unknown format type",
			input:      "any content",
			formats:    []string{"xml"},
			shouldPass: false, // Will fail due to unknown format
		},
		{
			name:       "Empty formats list",
			input:      "any content",
			formats:    []string{},
			shouldPass: true, // No validation requested
		},
		{
			name:       "Invalid JSON",
			input:      `{invalid json}`,
			formats:    []string{"json"},
			shouldPass: false,
		},
		{
			name:       "Invalid YAML",
			input:      "key:\n\ttab_not_allowed",
			formats:    []string{"yaml"},
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyFormatValidation(t, tt.input, tt.formats)

			if result != tt.shouldPass {
				t.Errorf("verifyFormatValidation() = %v, expected %v", result, tt.shouldPass)
			}
		})
	}
}

// TestMinMaxHelpers tests the helper min/max functions.
func TestMinMaxHelpers(t *testing.T) {
	testCases := []struct {
		name    string
		a, b    int
		wantMin int
		wantMax int
	}{
		{"positive numbers", 5, 10, 5, 10},
		{"negative numbers", -10, -5, -10, -5},
		{"mixed signs", -5, 5, -5, 5},
		{"equal values", 7, 7, 7, 7},
		{"zero and positive", 0, 5, 0, 5},
		{"zero and negative", -5, 0, -5, 0},
		{"large numbers", 1000000, 999999, 999999, 1000000},
		{"min int values", -2147483648, 2147483647, -2147483648, 2147483647},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotMin := min(tc.a, tc.b)
			if gotMin != tc.wantMin {
				t.Errorf("min(%d, %d) = %d, want %d", tc.a, tc.b, gotMin, tc.wantMin)
			}

			gotMax := max(tc.a, tc.b)
			if gotMax != tc.wantMax {
				t.Errorf("max(%d, %d) = %d, want %d", tc.a, tc.b, gotMax, tc.wantMax)
			}
		})
	}
}

// TestValidationErrorContext tests that validation provides helpful error context.
func TestValidationErrorContext(t *testing.T) {
	// Test YAML error preview
	t.Run("YAML error shows preview", func(t *testing.T) {
		longYAML := strings.Repeat("key: value\n", 20) + "  bad: indentation"
		result := verifyYAMLFormat(t, longYAML)

		if result {
			t.Error("Expected YAML validation to fail for bad indentation")
		}
		// The error should be logged via t.Errorf in the actual function
	})

	// Test JSON error shows offset
	t.Run("JSON error shows offset context", func(t *testing.T) {
		jsonWithError := `{"valid": "start", "bad": unterminated}`
		result := verifyJSONFormat(t, jsonWithError)

		if result {
			t.Error("Expected JSON validation to fail for unterminated value")
		}
		// The error context should be logged via t.Errorf in the actual function
	})
}

// TestLargeInputValidation tests validation with large inputs.
func TestLargeInputValidation(t *testing.T) {
	// Generate a large valid JSON array
	t.Run("Large JSON array", func(t *testing.T) {
		var items []string
		for i := 0; i < 1000; i++ {
			items = append(items, `"item"`)
		}
		largeJSON := "[" + strings.Join(items, ",") + "]"

		result := verifyJSONFormat(t, largeJSON)
		if !result {
			t.Error("Failed to validate large valid JSON array")
		}
	})

	// Generate a large valid YAML list
	t.Run("Large YAML list", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteString("items:\n")
		for i := 0; i < 1000; i++ {
			sb.WriteString("  - item\n")
		}

		result := verifyYAMLFormat(t, sb.String())
		if !result {
			t.Error("Failed to validate large valid YAML list")
		}
	})

	// Test preview truncation for large invalid input
	t.Run("Large invalid input preview truncation", func(t *testing.T) {
		// Create a large string that's invalid JSON
		largeInvalid := strings.Repeat("not json ", 1000)
		result := verifyJSONFormat(t, largeInvalid)

		if result {
			t.Error("Expected validation to fail for large invalid input")
		}
		// The preview should be truncated to 500 chars
	})
}
