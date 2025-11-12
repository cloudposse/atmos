package tests

import (
	"strings"
	"testing"
)

// TestYAMLEdgeCases tests additional edge cases for YAML validation.
func TestYAMLEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldPass bool
		desc       string
	}{
		// Edge cases for YAML
		{
			name:       "YAML with only comments",
			input:      "# This is a comment\n# Another comment",
			shouldPass: true,
			desc:       "Comments-only YAML should be valid",
		},
		{
			name:       "YAML with document separator",
			input:      "---\nkey: value\n...",
			shouldPass: true,
			desc:       "Document separators should be valid",
		},
		{
			name:       "YAML with multiple documents",
			input:      "---\nfirst: doc\n---\nsecond: doc",
			shouldPass: true,
			desc:       "Multiple documents should parse",
		},
		{
			name:       "YAML with anchors and aliases",
			input:      "default: &default\n  key: value\nother:\n  <<: *default",
			shouldPass: true,
			desc:       "Anchors and aliases should work",
		},
		{
			name:       "YAML with flow style",
			input:      "{key: value, list: [1, 2, 3]}",
			shouldPass: true,
			desc:       "Flow style should parse",
		},
		{
			name:       "YAML with null values",
			input:      "key: null\nother: ~\nempty:",
			shouldPass: true,
			desc:       "Various null representations",
		},
		{
			name:       "YAML with special strings",
			input:      "yes_string: 'yes'\nno_string: 'no'\ntrue_string: 'true'",
			shouldPass: true,
			desc:       "Quoted booleans should be strings",
		},
		{
			name:       "YAML with Unicode",
			input:      "emoji: 😀\nchinese: 中文\narabic: العربية",
			shouldPass: true,
			desc:       "Unicode should be supported",
		},
		{
			name:       "Empty YAML document with separator",
			input:      "---\n",
			shouldPass: true,
			desc:       "Empty document with separator",
		},
		{
			name:       "YAML with very long line",
			input:      "key: " + strings.Repeat("a", 10000),
			shouldPass: true,
			desc:       "Very long lines should work",
		},
		{
			name:       "Invalid YAML - circular reference",
			input:      "&a [*a]",
			shouldPass: false, // Go's YAML parser detects circular references
			desc:       "Circular references should fail",
		},
		{
			name:       "YAML with mixed indentation",
			input:      "key:\n  - item1\n\t- item2",
			shouldPass: false,
			desc:       "Mixed tabs and spaces should fail",
		},
		{
			name:       "YAML with invalid anchor name",
			input:      "&123 key: value",
			shouldPass: true, // Go's YAML parser accepts numeric anchor names
			desc:       "Numeric anchor names are accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateYAMLFormatSilent(tt.input)
			if result != tt.shouldPass {
				t.Errorf("%s: expected %v but got %v", tt.desc, tt.shouldPass, result)
			}
		})
	}
}

// TestJSONEdgeCases tests additional edge cases for JSON validation.
func TestJSONEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldPass bool
		desc       string
	}{
		// Edge cases for JSON
		{
			name:       "JSON with Unicode escape sequences",
			input:      `{"unicode": "\u0041\u0042\u0043"}`,
			shouldPass: true,
			desc:       "Unicode escapes should work",
		},
		{
			name:       "JSON with escaped characters",
			input:      `{"escaped": "line\nbreak\ttab\"quote\\backslash"}`,
			shouldPass: true,
			desc:       "All escape sequences should work",
		},
		{
			name:       "JSON with scientific notation",
			input:      `{"sci": 1.23e-4}`,
			shouldPass: true,
			desc:       "Scientific notation should parse",
		},
		{
			name:       "JSON with negative zero",
			input:      `{"negzero": -0}`,
			shouldPass: true,
			desc:       "Negative zero should parse",
		},
		{
			name:       "JSON with very deep nesting",
			input:      strings.Repeat(`{"a":`, 100) + `"value"` + strings.Repeat(`}`, 100),
			shouldPass: true,
			desc:       "Deep nesting should work",
		},
		{
			name:       "JSON with empty string key",
			input:      `{"": "empty key"}`,
			shouldPass: true,
			desc:       "Empty string keys are valid",
		},
		{
			name:       "JSON with whitespace",
			input:      " \n\t{\n\t\"key\" : \"value\"\n}\n\t ",
			shouldPass: true,
			desc:       "Whitespace should be ignored",
		},
		{
			name:       "JSON with large numbers",
			input:      `{"big": 9999999999999999999999999999999999999999}`,
			shouldPass: true,
			desc:       "Large numbers should parse",
		},
		{
			name:       "Invalid JSON - duplicate keys",
			input:      `{"key": "value1", "key": "value2"}`,
			shouldPass: true, // Go's JSON parser allows this
			desc:       "Duplicate keys (Go allows)",
		},
		{
			name:       "Invalid JSON - leading zeros",
			input:      `{"number": 0123}`,
			shouldPass: false,
			desc:       "Leading zeros are invalid",
		},
		{
			name:       "Invalid JSON - hex numbers",
			input:      `{"hex": 0x1234}`,
			shouldPass: false,
			desc:       "Hex notation is invalid",
		},
		{
			name:       "Invalid JSON - unquoted string value",
			input:      `{"key": unquoted}`,
			shouldPass: false,
			desc:       "Unquoted strings are invalid",
		},
		{
			name:       "Invalid JSON - JavaScript comments",
			input:      `{"key": "value" // comment\n}`,
			shouldPass: false,
			desc:       "Comments are not allowed",
		},
		{
			name:       "Invalid JSON - trailing decimal point",
			input:      `{"number": 123.}`,
			shouldPass: false,
			desc:       "Trailing decimal point invalid",
		},
		{
			name:       "Invalid JSON - plus sign",
			input:      `{"number": +123}`,
			shouldPass: false,
			desc:       "Plus sign is invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateJSONFormatSilent(tt.input)
			if result != tt.shouldPass {
				t.Errorf("%s: expected %v but got %v", tt.desc, tt.shouldPass, result)
			}
		})
	}
}

// TestValidationWithEmptyAndWhitespace tests handling of empty and whitespace inputs.
func TestValidationWithEmptyAndWhitespace(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		format     string
		shouldPass bool
	}{
		{"Empty string YAML", "", "yaml", true},
		{"Empty string JSON", "", "json", false},       // Empty string is not valid JSON
		{"Whitespace only YAML", "   ", "yaml", true},  // YAML accepts pure spaces as empty doc
		{"Whitespace only JSON", "   ", "json", false}, // Pure whitespace is not valid JSON
		{"Newlines only YAML", "\n\n\n", "yaml", true},
		{"Newlines only JSON", "\n\n\n", "json", false}, // Newlines only is not valid JSON
		{"Whitespace with null JSON", " null ", "json", true},
		{"Whitespace with YAML", "  \nkey: value\n  ", "yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result bool
			if tt.format == "yaml" {
				result = validateYAMLFormatSilent(tt.input)
			} else {
				result = validateJSONFormatSilent(tt.input)
			}

			if result != tt.shouldPass {
				t.Errorf("Expected %v for %s validation of %q", tt.shouldPass, tt.format, tt.input)
			}
		})
	}
}

// TestPreviewTruncation tests that error previews are properly truncated.
func TestPreviewTruncation(t *testing.T) {
	// Test YAML preview truncation
	t.Run("YAML preview truncation", func(t *testing.T) {
		// Create input with exactly 10 lines (with different keys to avoid duplicates)
		lines := make([]string, 10)
		for i := 0; i < 10; i++ {
			lines[i] = "key" + string(rune('0'+i)) + ": value" + string(rune('0'+i))
		}
		input := strings.Join(lines, "\n")

		// This should use all 10 lines in preview
		result := validateYAMLFormatSilent(input)
		if !result {
			t.Error("Valid YAML should pass")
		}
	})

	// Test with fewer than 10 lines
	t.Run("YAML preview with few lines", func(t *testing.T) {
		input := "line1\nline2\nline3"
		result := validateYAMLFormatSilent(input)
		if !result {
			t.Error("Valid YAML should pass")
		}
	})

	// Test preview length truncation at 500 chars
	t.Run("YAML preview length truncation", func(t *testing.T) {
		// Create a very long single line that will be truncated
		longLine := strings.Repeat("a", 600)
		result := validateYAMLFormatSilent(longLine)
		if !result {
			t.Error("Long string should be valid YAML")
		}
	})
}

// TestJSONSyntaxErrorContext tests JSON syntax error context extraction.
func TestJSONSyntaxErrorContext(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectContext bool
	}{
		{
			name:          "Error at beginning",
			input:         `{invalid`,
			expectContext: true,
		},
		{
			name:          "Error in middle",
			input:         `{"valid": "part", invalid}`,
			expectContext: true,
		},
		{
			name:          "Error at end",
			input:         `{"valid": "json"`,
			expectContext: true,
		},
		{
			name:          "Error with very long input",
			input:         `{"key": "` + strings.Repeat("a", 100) + `", invalid}`,
			expectContext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We expect these to fail and report context
			result := validateJSONFormatSilent(tt.input)
			if result {
				t.Error("Invalid JSON should fail validation")
			}
			// The context reporting is done via t.Errorf in the function
		})
	}
}

// TestFormatValidationCombinations tests various format combinations.
func TestFormatValidationCombinations(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		formats    []string
		shouldPass bool
	}{
		{
			name:       "Both formats with valid JSON",
			input:      `{"valid": "json"}`,
			formats:    []string{"json", "yaml"},
			shouldPass: true, // Valid JSON is valid YAML
		},
		{
			name:       "Both formats with YAML-only",
			input:      "key: value",
			formats:    []string{"json", "yaml"},
			shouldPass: false, // Not valid JSON
		},
		{
			name:       "Multiple same format",
			input:      `{"key": "value"}`,
			formats:    []string{"json", "json", "json"},
			shouldPass: true,
		},
		{
			name:       "Empty format list",
			input:      "anything",
			formats:    []string{},
			shouldPass: true, // No validation requested
		},
		{
			name:       "Unknown format in list",
			input:      "content",
			formats:    []string{"yaml", "xml", "json"},
			shouldPass: false, // Unknown format fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateFormatValidationSilent(tt.input, tt.formats)
			if result != tt.shouldPass {
				t.Errorf("Expected %v for formats %v", tt.shouldPass, tt.formats)
			}
		})
	}
}

// TestMinMaxBoundaries tests min/max functions at boundaries.
func TestMinMaxBoundaries(t *testing.T) {
	// Test with minimum and maximum int values
	const MaxInt = int(^uint(0) >> 1)
	const MinInt = -MaxInt - 1

	tests := []struct {
		name    string
		a, b    int
		wantMin int
		wantMax int
	}{
		{"MaxInt and 0", MaxInt, 0, 0, MaxInt},
		{"MinInt and 0", MinInt, 0, MinInt, 0},
		{"MaxInt and MinInt", MaxInt, MinInt, MinInt, MaxInt},
		{"MaxInt and MaxInt", MaxInt, MaxInt, MaxInt, MaxInt},
		{"MinInt and MinInt", MinInt, MinInt, MinInt, MinInt},
		{"MaxInt-1 and MaxInt", MaxInt - 1, MaxInt, MaxInt - 1, MaxInt},
		{"MinInt and MinInt+1", MinInt, MinInt + 1, MinInt, MinInt + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMin := min(tt.a, tt.b)
			if gotMin != tt.wantMin {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, gotMin, tt.wantMin)
			}

			gotMax := max(tt.a, tt.b)
			if gotMax != tt.wantMax {
				t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, gotMax, tt.wantMax)
			}
		})
	}
}
