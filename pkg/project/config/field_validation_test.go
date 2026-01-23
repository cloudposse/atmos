package config

import (
	"testing"
)

func TestCreateField_InvalidFieldType(t *testing.T) {
	// Test that unknown field types cause a panic with a helpful message
	field := FieldDefinition{
		Type:        "invalid_type",
		Label:       "Test Field",
		Description: "This should fail",
		Default:     "test",
	}

	values := map[string]interface{}{
		"test_field": "test_value",
	}

	// Expect a panic for invalid field type
	defer func() {
		if r := recover(); r != nil {
			expectedMessage := "unsupported field type 'invalid_type' for field 'test_field'. Supported types: input, text, string, select, multiselect, confirm, bool, boolean"
			if r != expectedMessage {
				t.Errorf("Expected panic message %q, got %q", expectedMessage, r)
			}
		} else {
			t.Error("Expected createField to panic for invalid field type, but it didn't")
		}
	}()

	createField("test_field", &field, values)
}

func TestCreateField_ValidFieldTypes(t *testing.T) {
	values := map[string]interface{}{
		"string_field":      "test_string",
		"select_field":      "option1",
		"multiselect_field": []string{"option1", "option2"},
		"confirm_field":     true,
	}

	testCases := []struct {
		name      string
		fieldType string
		field     FieldDefinition
		key       string
	}{
		{
			name:      "input type",
			fieldType: "input",
			field: FieldDefinition{
				Type:        "input",
				Label:       "Input Field",
				Description: "Test input field",
				Default:     "default_value",
			},
			key: "string_field",
		},
		{
			name:      "text type",
			fieldType: "text",
			field: FieldDefinition{
				Type:        "text",
				Label:       "Text Field",
				Description: "Test text field",
				Default:     "default_value",
			},
			key: "string_field",
		},
		{
			name:      "string type",
			fieldType: "string",
			field: FieldDefinition{
				Type:        "string",
				Label:       "String Field",
				Description: "Test string field",
				Default:     "default_value",
			},
			key: "string_field",
		},
		{
			name:      "select type",
			fieldType: "select",
			field: FieldDefinition{
				Type:        "select",
				Label:       "Select Field",
				Description: "Test select field",
				Options:     []string{"option1", "option2", "option3"},
				Default:     "option1",
			},
			key: "select_field",
		},
		{
			name:      "multiselect type",
			fieldType: "multiselect",
			field: FieldDefinition{
				Type:        "multiselect",
				Label:       "Multiselect Field",
				Description: "Test multiselect field",
				Options:     []string{"option1", "option2", "option3"},
				Default:     []string{"option1"},
			},
			key: "multiselect_field",
		},
		{
			name:      "confirm type",
			fieldType: "confirm",
			field: FieldDefinition{
				Type:        "confirm",
				Label:       "Confirm Field",
				Description: "Test confirm field",
				Default:     true,
			},
			key: "confirm_field",
		},
		{
			name:      "bool type",
			fieldType: "bool",
			field: FieldDefinition{
				Type:        "bool",
				Label:       "Bool Field",
				Description: "Test bool field",
				Default:     false,
			},
			key: "confirm_field",
		},
		{
			name:      "boolean type",
			fieldType: "boolean",
			field: FieldDefinition{
				Type:        "boolean",
				Label:       "Boolean Field",
				Description: "Test boolean field",
				Default:     true,
			},
			key: "confirm_field",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic for valid field types
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("createField panicked for valid field type %s: %v", tc.fieldType, r)
				}
			}()

			field, getter := createField(tc.key, &tc.field, values)
			if field == nil {
				t.Errorf("createField returned nil field for type %s", tc.fieldType)
			}
			if getter == nil {
				t.Errorf("createField returned nil getter for type %s", tc.fieldType)
			}

			// Test that getter returns the expected type
			value := getter()
			switch tc.fieldType {
			case "input", "text", "string", "select":
				if _, ok := value.(string); !ok {
					t.Errorf("Expected string value for type %s, got %T", tc.fieldType, value)
				}
			case "multiselect":
				if _, ok := value.([]string); !ok {
					t.Errorf("Expected []string value for type %s, got %T", tc.fieldType, value)
				}
			case "confirm", "bool", "boolean":
				if _, ok := value.(bool); !ok {
					t.Errorf("Expected bool value for type %s, got %T", tc.fieldType, value)
				}
			}
		})
	}
}

func TestCreateField_BooleanTypesAllWorkAsSame(t *testing.T) {
	// Test that all boolean type variations (confirm, bool, boolean) work the same way
	values := map[string]interface{}{
		"test_field": true,
	}

	booleanTypes := []string{"confirm", "bool", "boolean"}

	for _, boolType := range booleanTypes {
		t.Run(boolType, func(t *testing.T) {
			field := FieldDefinition{
				Type:        boolType,
				Label:       "Boolean Field",
				Description: "Test boolean field",
				Default:     false,
			}

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("createField panicked for boolean type %s: %v", boolType, r)
				}
			}()

			huhField, getter := createField("test_field", &field, values)
			if huhField == nil {
				t.Errorf("createField returned nil field for boolean type %s", boolType)
			}
			if getter == nil {
				t.Errorf("createField returned nil getter for boolean type %s", boolType)
			}

			// All boolean types should return bool values
			value := getter()
			if _, ok := value.(bool); !ok {
				t.Errorf("Expected bool value for type %s, got %T", boolType, value)
			}
		})
	}
}
