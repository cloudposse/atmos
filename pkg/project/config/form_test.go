package config

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/stretchr/testify/assert"
)

func TestPromptForScaffoldConfig_FormCreation(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test that forms can be created with different field types
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Test Scaffold Template",
		Description: "Test scaffold template configuration",
		Fields: map[string]FieldDefinition{
			"name": {
				Key:         "name",
				Type:        "input",
				Label:       "Project Name",
				Description: "The name of your project",
				Default:     "my-project",
				Required:    true,
				Placeholder: "Enter project name",
			},
			"license": {
				Key:         "license",
				Type:        "select",
				Label:       "License",
				Description: "Choose a license",
				Default:     "MIT",
				Required:    true,
				Options:     []string{"MIT", "Apache", "GPL"},
			},
			"regions": {
				Key:         "regions",
				Type:        "multiselect",
				Label:       "AWS Regions",
				Description: "Select regions",
				Default:     []string{"us-east-1"},
				Required:    true,
				Options:     []string{"us-east-1", "us-west-2", "eu-west-1"},
			},
			"monitoring": {
				Key:         "monitoring",
				Type:        "confirm",
				Label:       "Enable Monitoring",
				Description: "Enable monitoring and alerting",
				Default:     true,
				Required:    false,
			},
		},
	}

	userValues := map[string]interface{}{
		"name":       "test-project",
		"license":    "Apache",
		"regions":    []string{"us-east-1", "us-west-2"},
		"monitoring": true,
	}

	// Test that the function doesn't panic
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})
}

func TestPromptForScaffoldConfig_FieldGrouping(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test that fields are properly grouped by type
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Test Scaffold Template",
		Description: "Test scaffold template configuration",
		Fields: map[string]FieldDefinition{
			// Basic fields (input, text)
			"name": {
				Key:      "name",
				Type:     "input",
				Label:    "Project Name",
				Required: true,
			},
			"description": {
				Key:      "description",
				Type:     "text",
				Label:    "Description",
				Required: false,
			},
			// Config fields (select)
			"license": {
				Key:      "license",
				Type:     "select",
				Label:    "License",
				Required: true,
				Options:  []string{"MIT", "Apache"},
			},
			"cloud_provider": {
				Key:      "cloud_provider",
				Type:     "select",
				Label:    "Cloud Provider",
				Required: true,
				Options:  []string{"aws", "gcp", "azure"},
			},
			// Advanced fields (multiselect, confirm)
			"regions": {
				Key:      "regions",
				Type:     "multiselect",
				Label:    "Regions",
				Required: true,
				Options:  []string{"us-east-1", "us-west-2"},
			},
			"monitoring": {
				Key:      "monitoring",
				Type:     "confirm",
				Label:    "Enable Monitoring",
				Required: false,
			},
		},
	}

	userValues := make(map[string]interface{})

	// Test that the function doesn't panic with complex field grouping
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})
}

func TestPromptForScaffoldConfig_Validation(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test form validation
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Test Scaffold Template",
		Description: "Test scaffold template configuration",
		Fields: map[string]FieldDefinition{
			"required_field": {
				Key:      "required_field",
				Type:     "input",
				Label:    "Required Field",
				Required: true,
			},
			"optional_field": {
				Key:      "optional_field",
				Type:     "input",
				Label:    "Optional Field",
				Required: false,
			},
		},
	}

	userValues := map[string]interface{}{
		"required_field": "has value",
		"optional_field": "",
	}

	// Test that the function doesn't panic with validation
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})
}

func TestPromptForScaffoldConfig_DefaultValues(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test that default values are properly handled
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Test Scaffold Template",
		Description: "Test scaffold template configuration",
		Fields: map[string]FieldDefinition{
			"string_default": {
				Key:     "string_default",
				Type:    "input",
				Label:   "String Default",
				Default: "default string",
			},
			"bool_default": {
				Key:     "bool_default",
				Type:    "confirm",
				Label:   "Bool Default",
				Default: true,
			},
			"array_default": {
				Key:     "array_default",
				Type:    "multiselect",
				Label:   "Array Default",
				Default: []string{"default1", "default2"},
				Options: []string{"default1", "default2", "option3"},
			},
		},
	}

	userValues := make(map[string]interface{})

	// Test that the function doesn't panic with default values
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})
}

func TestPromptForScaffoldConfig_ValueCapture(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test that form values are properly captured
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Test Scaffold Template",
		Description: "Test scaffold template configuration",
		Fields: map[string]FieldDefinition{
			"name": {
				Key:     "name",
				Type:    "input",
				Label:   "Project Name",
				Default: "default-project",
			},
			"license": {
				Key:     "license",
				Type:    "select",
				Label:   "License",
				Default: "MIT",
				Options: []string{"MIT", "Apache"},
			},
		},
	}

	userValues := map[string]interface{}{
		"name":    "user-project",
		"license": "Apache",
	}

	// Test that the function doesn't panic and values are captured
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})

	// Verify that user values override defaults
	assert.Equal(t, "user-project", userValues["name"])
	assert.Equal(t, "Apache", userValues["license"])
}

func TestPromptForScaffoldConfig_EmptyConfig(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Test with empty scaffold config
	scaffoldConfig := &ScaffoldConfig{
		Name:        "Empty Project",
		Description: "Empty scaffold template configuration",
		Fields:      map[string]FieldDefinition{},
	}

	userValues := make(map[string]interface{})

	// Test that the function doesn't panic with empty config
	assert.NotPanics(t, func() {
		PromptForScaffoldConfig(scaffoldConfig, userValues)
	})
}

func TestPromptForScaffoldConfig_AllFieldsCaptured(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Create a scaffold config with all field types
	scaffoldConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"name": {
				Key:         "name",
				Type:        "input",
				Label:       "Project Name",
				Default:     "default-project",
				Required:    true,
				Placeholder: "Enter project name",
			},
			"author": {
				Key:         "author",
				Type:        "input",
				Label:       "Author",
				Default:     "Default Author",
				Required:    true,
				Placeholder: "Enter author name",
			},
			"year": {
				Key:         "year",
				Type:        "input",
				Label:       "Year",
				Default:     "2024",
				Required:    true,
				Placeholder: "Enter year",
			},
			"license": {
				Key:      "license",
				Type:     "select",
				Label:    "License",
				Default:  "MIT",
				Required: true,
				Options:  []string{"MIT", "Apache", "GPL"},
			},
			"regions": {
				Key:      "regions",
				Type:     "multiselect",
				Label:    "Regions",
				Default:  []string{"us-east-1"},
				Required: true,
				Options:  []string{"us-east-1", "us-west-2", "eu-west-1"},
			},
			"enable_monitoring": {
				Key:      "enable_monitoring",
				Type:     "confirm",
				Label:    "Enable Monitoring",
				Default:  true,
				Required: false,
			},
		},
	}

	// Initial user values
	userValues := map[string]interface{}{
		"name":              "test-project",
		"author":            "Test Author",
		"year":              "2025",
		"license":           "Apache",
		"regions":           []string{"us-west-2"},
		"enable_monitoring": false,
	}

	// This test verifies that all fields are properly captured
	// In a real interactive test, we would need to mock the form input
	err := PromptForScaffoldConfig(scaffoldConfig, userValues)

	// The function should either complete successfully or return an error
	// but it shouldn't crash
	if err != nil && !strings.Contains(err.Error(), "user aborted") {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify that all expected fields are present in userValues
	expectedFields := []string{"name", "author", "year", "license", "regions", "enable_monitoring"}
	for _, field := range expectedFields {
		if _, exists := userValues[field]; !exists {
			t.Errorf("Field '%s' is missing from userValues", field)
		}
	}
}

func TestCreateField_AllFieldTypes(t *testing.T) {
	// Test all field types to ensure they're created correctly
	testCases := []struct {
		name     string
		field    FieldDefinition
		expected string
	}{
		{
			name: "input field",
			field: FieldDefinition{
				Key:         "name",
				Type:        "input",
				Label:       "Project Name",
				Default:     "default-project",
				Required:    true,
				Placeholder: "Enter project name",
			},
			expected: "input",
		},
		{
			name: "select field",
			field: FieldDefinition{
				Key:      "license",
				Type:     "select",
				Label:    "License",
				Default:  "MIT",
				Required: true,
				Options:  []string{"MIT", "Apache", "GPL"},
			},
			expected: "select",
		},
		{
			name: "multiselect field",
			field: FieldDefinition{
				Key:      "regions",
				Type:     "multiselect",
				Label:    "Regions",
				Default:  []string{"us-east-1"},
				Required: true,
				Options:  []string{"us-east-1", "us-west-2"},
			},
			expected: "multiselect",
		},
		{
			name: "confirm field",
			field: FieldDefinition{
				Key:      "enable_monitoring",
				Type:     "confirm",
				Label:    "Enable Monitoring",
				Default:  true,
				Required: false,
			},
			expected: "confirm",
		},
		{
			name: "year field",
			field: FieldDefinition{
				Key:         "year",
				Type:        "input",
				Label:       "Year",
				Default:     "2024",
				Required:    true,
				Placeholder: "2024",
			},
			expected: "input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			values := make(map[string]interface{})

			// Set initial value
			values[tc.field.Key] = tc.field.Default

			// Create the field
			field, _ := createField(tc.field.Key, &tc.field, values)

			// Verify the field was created (we can't easily test the exact type without reflection)
			// but we can verify it's not nil
			if field == nil {
				t.Errorf("Field was not created for %s", tc.name)
			}

			// Verify the value is still in the map
			if _, exists := values[tc.field.Key]; !exists {
				t.Errorf("Value for field '%s' was removed from values map", tc.field.Key)
			}
		})
	}
}

func TestCreateField_ValueOverwriteIssue(t *testing.T) {
	// This test reproduces the issue where values are being overwritten
	// in the createField function before the form runs

	field := FieldDefinition{
		Key:         "year",
		Type:        "input",
		Label:       "Year",
		Default:     "2024",
		Required:    true,
		Placeholder: "2024",
	}

	// Simulate the values map that would be passed to createField
	values := make(map[string]interface{})
	values["year"] = "2025" // User-provided value

	// Create the field - this should NOT overwrite the value
	_, _ = createField("year", &field, values)

	// The value should still be "2025", not "2024"
	if values["year"] != "2025" {
		t.Errorf("Expected year to remain '2025', but got '%v'", values["year"])
	}

	// Test with a different field type
	field2 := FieldDefinition{
		Key:      "license",
		Type:     "select",
		Label:    "License",
		Default:  "MIT",
		Required: true,
		Options:  []string{"MIT", "Apache", "GPL"},
	}

	values["license"] = "Apache" // User-provided value

	// Create the field - this should NOT overwrite the value
	_, _ = createField("license", &field2, values)

	// The value should still be "Apache", not "MIT"
	if values["license"] != "Apache" {
		t.Errorf("Expected license to remain 'Apache', but got '%v'", values["license"])
	}
}

func TestCreateField_UserInputPriority(t *testing.T) {
	// This test verifies that user input is prioritized over defaults

	field := FieldDefinition{
		Key:         "year",
		Type:        "input",
		Label:       "Year",
		Default:     "2024",
		Required:    true,
		Placeholder: "2024",
	}

	// Simulate user-provided value
	values := make(map[string]interface{})
	values["year"] = "2025" // User wants 2025

	// Create the field
	fieldObj, getter := createField("year", &field, values)

	// The field should be created
	if fieldObj == nil {
		t.Errorf("Field was not created")
	}

	// The getter should return the user's value, not the default
	value := getter()
	if value != "2025" {
		t.Errorf("Expected getter to return user value '2025', but got '%v'", value)
	}

	// The values map should still contain the user's value
	if values["year"] != "2025" {
		t.Errorf("Expected values map to contain user value '2025', but got '%v'", values["year"])
	}
}

func TestPromptForScaffoldConfig_UserInputCapture(t *testing.T) {
	// This test simulates the complete form flow to verify user input is captured

	scaffoldConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"year": {
				Key:         "year",
				Type:        "input",
				Label:       "Year",
				Default:     "2024",
				Required:    true,
				Placeholder: "2024",
			},
			"author": {
				Key:         "author",
				Type:        "input",
				Label:       "Author",
				Default:     "Default Author",
				Required:    true,
				Placeholder: "Enter author name",
			},
		},
	}

	// Initial user values (could be from command line or previous run)
	userValues := map[string]interface{}{
		"year":   "2025", // User wants 2025
		"author": "John Doe",
	}

	// This simulates what happens in the real form
	// We can't easily test the interactive part, but we can verify the setup

	// Initialize form values with user values and defaults
	formValues := make(map[string]interface{})

	// Set defaults from scaffold config
	for key, field := range scaffoldConfig.Fields {
		if field.Default != nil {
			formValues[key] = field.Default
		}
	}

	// Override with user values
	for key, value := range userValues {
		formValues[key] = value
	}

	// Verify that user values take priority
	if formValues["year"] != "2025" {
		t.Errorf("Expected year to be '2025' (user value), but got '%v'", formValues["year"])
	}

	if formValues["author"] != "John Doe" {
		t.Errorf("Expected author to be 'John Doe' (user value), but got '%v'", formValues["author"])
	}

	// Test that the createField function uses the correct values
	yearFieldDef := scaffoldConfig.Fields["year"]
	yearField, yearGetter := createField("year", &yearFieldDef, formValues)
	if yearField == nil {
		t.Errorf("Year field was not created")
	}

	// The getter should return the user's value
	yearValue := yearGetter()
	if yearValue != "2025" {
		t.Errorf("Expected year getter to return '2025', but got '%v'", yearValue)
	}

	// Test author field
	authorFieldDef := scaffoldConfig.Fields["author"]
	authorField, authorGetter := createField("author", &authorFieldDef, formValues)
	if authorField == nil {
		t.Errorf("Author field was not created")
	}

	authorValue := authorGetter()
	if authorValue != "John Doe" {
		t.Errorf("Expected author getter to return 'John Doe', but got '%v'", authorValue)
	}
}

func TestPromptForScaffoldConfig_ExistingValuesPriority(t *testing.T) {
	// This test verifies that existing values from scaffold.yaml take priority over defaults

	scaffoldConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"year": {
				Key:         "year",
				Type:        "input",
				Label:       "Year",
				Default:     "2024",
				Required:    true,
				Placeholder: "2024",
			},
			"author": {
				Key:         "author",
				Type:        "input",
				Label:       "Author",
				Default:     "Default Author",
				Required:    true,
				Placeholder: "Enter author name",
			},
		},
	}

	// Simulate existing values from scaffold.yaml (what would be loaded by LoadUserValues)
	existingValues := map[string]interface{}{
		"year":   "2025",   // Existing value from scaffold.yaml
		"author": "Foobar", // Existing value from scaffold.yaml
	}

	// This simulates what happens in executeWithSetup
	// Deep merge project defaults with user values
	mergedValues := DeepMerge(scaffoldConfig, existingValues)

	// Verify that existing values take priority over defaults
	if mergedValues["year"] != "2025" {
		t.Errorf("Expected year to be '2025' (existing value), but got '%v'", mergedValues["year"])
	}

	if mergedValues["author"] != "Foobar" {
		t.Errorf("Expected author to be 'Foobar' (existing value), but got '%v'", mergedValues["author"])
	}

	// Now simulate what happens in PromptForScaffoldConfig
	formValues := make(map[string]interface{})

	// Set defaults from scaffold config
	for key, field := range scaffoldConfig.Fields {
		if field.Default != nil {
			formValues[key] = field.Default
		}
	}

	// Override with user values (mergedValues contains existing values)
	for key, value := range mergedValues {
		formValues[key] = value
	}

	// Verify that existing values take priority in form values
	if formValues["year"] != "2025" {
		t.Errorf("Expected formValues year to be '2025' (existing value), but got '%v'", formValues["year"])
	}

	if formValues["author"] != "Foobar" {
		t.Errorf("Expected formValues author to be 'Foobar' (existing value), but got '%v'", formValues["author"])
	}

	// Test that createField uses the correct values
	yearFieldDef := scaffoldConfig.Fields["year"]
	yearField, yearGetter := createField("year", &yearFieldDef, formValues)
	if yearField == nil {
		t.Errorf("Year field was not created")
	}

	yearValue := yearGetter()
	if yearValue != "2025" {
		t.Errorf("Expected year getter to return '2025' (existing value), but got '%v'", yearValue)
	}

	authorFieldDef := scaffoldConfig.Fields["author"]
	authorField, authorGetter := createField("author", &authorFieldDef, formValues)
	if authorField == nil {
		t.Errorf("Author field was not created")
	}

	authorValue := authorGetter()
	if authorValue != "Foobar" {
		t.Errorf("Expected author getter to return 'Foobar' (existing value), but got '%v'", authorValue)
	}
}
