package config

import (
	"testing"
)

// TestInitializeFormValues tests the initializeFormValues function.
func TestInitializeFormValues(t *testing.T) {
	tests := []struct {
		name           string
		scaffoldConfig *ScaffoldConfig
		userValues     map[string]interface{}
		expected       map[string]interface{}
	}{
		{
			name: "defaults only",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name":    {Default: "default-name"},
					"version": {Default: "1.0.0"},
				},
			},
			userValues: map[string]interface{}{},
			expected: map[string]interface{}{
				"name":    "default-name",
				"version": "1.0.0",
			},
		},
		{
			name: "user values override defaults",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name":    {Default: "default-name"},
					"version": {Default: "1.0.0"},
				},
			},
			userValues: map[string]interface{}{
				"name": "custom-name",
			},
			expected: map[string]interface{}{
				"name":    "custom-name",
				"version": "1.0.0",
			},
		},
		{
			name: "nil defaults are not included",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name":        {Default: "default-name"},
					"description": {Default: nil},
				},
			},
			userValues: map[string]interface{}{},
			expected: map[string]interface{}{
				"name": "default-name",
			},
		},
		{
			name: "user values with no defaults",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name": {Default: nil},
				},
			},
			userValues: map[string]interface{}{
				"name": "user-name",
			},
			expected: map[string]interface{}{
				"name": "user-name",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := initializeFormValues(tt.scaffoldConfig, tt.userValues)

			// Check all expected keys
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("Expected %s=%v, got %v", key, expectedValue, result[key])
				}
			}

			// Verify no unexpected keys
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(result))
			}
		})
	}
}

// TestGroupFieldsByType tests the groupFieldsByType function.
func TestGroupFieldsByType(t *testing.T) {
	tests := []struct {
		name             string
		scaffoldConfig   *ScaffoldConfig
		expectedBasic    int
		expectedConfig   int
		expectedAdvanced int
	}{
		{
			name: "mixed field types",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name":        {Type: "input"},
					"description": {Type: "text"},
					"env":         {Type: "select"},
					"features":    {Type: "multiselect"},
					"enabled":     {Type: "confirm"},
				},
			},
			expectedBasic:    2, // input, text
			expectedConfig:   1, // select
			expectedAdvanced: 2, // multiselect, confirm
		},
		{
			name: "all basic fields",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"field1": {Type: "input"},
					"field2": {Type: "text"},
					"field3": {Type: "input"},
				},
			},
			expectedBasic:    3,
			expectedConfig:   0,
			expectedAdvanced: 0,
		},
		{
			name: "unknown type defaults to basic",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"field1": {Type: "unknown"},
					"field2": {Type: "input"},
				},
			},
			expectedBasic:    2,
			expectedConfig:   0,
			expectedAdvanced: 0,
		},
		{
			name: "empty fields",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{},
			},
			expectedBasic:    0,
			expectedConfig:   0,
			expectedAdvanced: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			basic, config, advanced := groupFieldsByType(tt.scaffoldConfig)

			if len(basic) != tt.expectedBasic {
				t.Errorf("Expected %d basic fields, got %d", tt.expectedBasic, len(basic))
			}
			if len(config) != tt.expectedConfig {
				t.Errorf("Expected %d config fields, got %d", tt.expectedConfig, len(config))
			}
			if len(advanced) != tt.expectedAdvanced {
				t.Errorf("Expected %d advanced fields, got %d", tt.expectedAdvanced, len(advanced))
			}
		})
	}
}

// TestExtractFormValues tests the extractFormValues function.
func TestExtractFormValues(t *testing.T) {
	tests := []struct {
		name         string
		userValues   map[string]interface{}
		valueGetters map[string]func() interface{}
		expected     map[string]interface{}
	}{
		{
			name:       "extract string values",
			userValues: make(map[string]interface{}),
			valueGetters: map[string]func() interface{}{
				"name":    func() interface{} { return "test-name" },
				"version": func() interface{} { return "1.0.0" },
			},
			expected: map[string]interface{}{
				"name":    "test-name",
				"version": "1.0.0",
			},
		},
		{
			name:       "extract mixed types",
			userValues: make(map[string]interface{}),
			valueGetters: map[string]func() interface{}{
				"name":     func() interface{} { return "test" },
				"enabled":  func() interface{} { return true },
				"features": func() interface{} { return []string{"a", "b"} },
			},
			expected: map[string]interface{}{
				"name":     "test",
				"enabled":  true,
				"features": []string{"a", "b"},
			},
		},
		{
			name:         "empty getters",
			userValues:   make(map[string]interface{}),
			valueGetters: map[string]func() interface{}{},
			expected:     map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractFormValues(tt.userValues, tt.valueGetters)

			// Verify extracted values
			for key, expectedValue := range tt.expected {
				actualValue, exists := tt.userValues[key]
				if !exists {
					t.Errorf("Expected key %s not found", key)
					continue
				}

				// Use type-specific comparisons
				switch expected := expectedValue.(type) {
				case string:
					if actual, ok := actualValue.(string); !ok || actual != expected {
						t.Errorf("Expected %s=%v, got %v", key, expected, actual)
					}
				case bool:
					if actual, ok := actualValue.(bool); !ok || actual != expected {
						t.Errorf("Expected %s=%v, got %v", key, expected, actual)
					}
				case []string:
					actual, ok := actualValue.([]string)
					if !ok {
						t.Errorf("Expected %s to be []string, got %T", key, actualValue)
						continue
					}
					if len(actual) != len(expected) {
						t.Errorf("Expected %s length %d, got %d", key, len(expected), len(actual))
						continue
					}
					for i := range expected {
						if actual[i] != expected[i] {
							t.Errorf("Expected %s[%d]=%v, got %v", key, i, expected[i], actual[i])
						}
					}
				}
			}

			// Verify no unexpected keys
			if len(tt.userValues) != len(tt.expected) {
				t.Errorf("Expected %d keys, got %d", len(tt.expected), len(tt.userValues))
			}
		})
	}
}

// TestBuildConfigForm tests the buildConfigForm function.
func TestBuildConfigForm(t *testing.T) {
	tests := []struct {
		name               string
		scaffoldConfig     *ScaffoldConfig
		formValues         map[string]interface{}
		expectedGettersLen int
	}{
		{
			name: "basic form with mixed fields",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{
					"name":     {Type: "input", Default: "test"},
					"env":      {Type: "select", Options: []string{"dev", "prod"}},
					"features": {Type: "multiselect", Options: []string{"a", "b"}},
				},
			},
			formValues:         map[string]interface{}{},
			expectedGettersLen: 3,
		},
		{
			name: "empty scaffold config",
			scaffoldConfig: &ScaffoldConfig{
				Fields: map[string]FieldDefinition{},
			},
			formValues:         map[string]interface{}{},
			expectedGettersLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form, getters := buildConfigForm(tt.scaffoldConfig, tt.formValues)

			if form == nil {
				t.Error("Expected non-nil form")
			}

			if len(getters) != tt.expectedGettersLen {
				t.Errorf("Expected %d getters, got %d", tt.expectedGettersLen, len(getters))
			}
		})
	}
}

// TestCreateFormGroup tests the createFormGroup function.
func TestCreateFormGroup(t *testing.T) {
	tests := []struct {
		name               string
		items              []fieldItem
		formValues         map[string]interface{}
		expectedFieldCount int
	}{
		{
			name: "create group with multiple fields",
			items: []fieldItem{
				{key: "name", field: FieldDefinition{Type: "input", Default: "test"}},
				{key: "version", field: FieldDefinition{Type: "input", Default: "1.0.0"}},
			},
			formValues:         map[string]interface{}{},
			expectedFieldCount: 2,
		},
		{
			name: "create group with single field",
			items: []fieldItem{
				{key: "name", field: FieldDefinition{Type: "input", Default: "test"}},
			},
			formValues:         map[string]interface{}{},
			expectedFieldCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valueGetters := make(map[string]func() interface{})
			group := createFormGroup(tt.items, tt.formValues, valueGetters)

			if group == nil {
				t.Error("Expected non-nil group")
			}

			if len(valueGetters) != tt.expectedFieldCount {
				t.Errorf("Expected %d value getters, got %d", tt.expectedFieldCount, len(valueGetters))
			}
		})
	}
}
