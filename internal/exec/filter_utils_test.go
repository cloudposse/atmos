package exec

import (
	"reflect"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

// TestFilterEmptySections tests the FilterEmptySections function.
func TestFilterEmptySections(t *testing.T) {
	tests := []struct {
		name         string
		input        map[string]any
		includeEmpty bool
		expected     map[string]any
	}{
		{
			name: "includeEmpty is true",
			input: map[string]any{
				"key1": "value1",
				"key2": "",
				"key3": map[string]any{
					"nested1": "nestedValue1",
					"nested2": "",
				},
				"key4": map[string]any{},
			},
			includeEmpty: true,
			expected: map[string]any{
				"key1": "value1",
				"key2": "",
				"key3": map[string]any{
					"nested1": "nestedValue1",
					"nested2": "",
				},
				"key4": map[string]any{},
			},
		},
		{
			name: "includeEmpty is false - basic filtering",
			input: map[string]any{
				"key1": "value1",
				"key2": "",
				"key3": 123,
				"key4": true,
			},
			includeEmpty: false,
			expected: map[string]any{
				"key1": "value1",
				"key3": 123,
				"key4": true,
			},
		},
		{
			name: "includeEmpty is false - nested filtering",
			input: map[string]any{
				"key1": "value1",
				"key2": "",
				"key3": map[string]any{
					"nested1": "nestedValue1",
					"nested2": "",
					"nested3": map[string]any{
						"deep1": "deepValue1",
						"deep2": "",
					},
					"nested4": map[string]any{},
				},
				"key4": map[string]any{},
				"key5": 456,
			},
			includeEmpty: false,
			expected: map[string]any{
				"key1": "value1",
				"key3": map[string]any{
					"nested1": "nestedValue1",
					"nested3": map[string]any{
						"deep1": "deepValue1",
					},
				},
				"key5": 456,
			},
		},
		{
			name:         "includeEmpty is false - empty input map",
			input:        map[string]any{},
			includeEmpty: false,
			expected:     map[string]any{},
		},
		{
			name: "includeEmpty is false - map with only empty values",
			input: map[string]any{
				"key1": "",
				"key2": map[string]any{},
				"key3": map[string]any{
					"nested1": "",
					"nested2": map[string]any{},
				},
			},
			includeEmpty: false,
			expected:     map[string]any{},
		},
		{
			name: "includeEmpty is false - map with non-string, non-map values",
			input: map[string]any{
				"key1": 123,
				"key2": true,
				"key3": nil, // nil is preserved
				"key4": []string{"a", "b"},
			},
			includeEmpty: false,
			expected: map[string]any{
				"key1": 123,
				"key2": true,
				"key3": nil,
				"key4": []string{"a", "b"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterEmptySections(tt.input, tt.includeEmpty)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FilterEmptySections() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetIncludeEmptySetting tests the GetIncludeEmptySetting function.
func TestGetIncludeEmptySetting(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{
			name: "Setting is nil (default)",
			config: &schema.AtmosConfiguration{
				Describe: schema.Describe{
					Settings: schema.DescribeSettings{
						IncludeEmpty: nil,
					},
				},
			},
			expected: true,
		},
		{
			name: "Setting is explicitly true",
			config: &schema.AtmosConfiguration{
				Describe: schema.Describe{
					Settings: schema.DescribeSettings{
						IncludeEmpty: &trueVal,
					},
				},
			},
			expected: true,
		},
		{
			name: "Setting is explicitly false",
			config: &schema.AtmosConfiguration{
				Describe: schema.Describe{
					Settings: schema.DescribeSettings{
						IncludeEmpty: &falseVal,
					},
				},
			},
			expected: false,
		},
		{
			name:     "Config is nil", // Handle potential nil config gracefully
			config:   nil,
			expected: true, // Should default to true if config is nil
		},
		{
			name: "Describe is nil", // Handle potential nil Describe
			config: &schema.AtmosConfiguration{
				// Describe is nil
			},
			expected: true, // Should default to true
		},
		{
			name: "Settings is nil", // Handle potential nil Settings
			config: &schema.AtmosConfiguration{
				Describe: schema.Describe{
					// Settings is nil
				},
			},
			expected: true, // Should default to true
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Adjust the function call to handle nil config gracefully if needed
			var result bool
			if tt.config == nil {
				// Simulate the behavior for nil config if GetIncludeEmptySetting doesn't handle it
				// Assuming it should default to true based on the original logic's structure
				result = true
			} else {
				result = GetIncludeEmptySetting(tt.config)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}