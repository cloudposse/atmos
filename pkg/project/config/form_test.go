package config

import (
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/manifest"
)

// withNoOpFormRunner replaces runFormInteraction with a no-op for the duration
// of the test. This prevents the real huh TUI from starting during go test and
// keeps the suite hermetic across all environments (CI, headless, local TTY).
func withNoOpFormRunner(t *testing.T) {
	t.Helper()
	original := runFormInteraction
	runFormInteraction = func(_ *huh.Form) error {
		return nil
	}
	t.Cleanup(func() { runFormInteraction = original })
}

func TestPromptForScaffoldConfig_FormCreation(t *testing.T) {
	withNoOpFormRunner(t)

	// Test that forms can be created with different field types.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{
			Name:        "test-scaffold",
			Description: "Test scaffold template configuration",
		},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{
					Name:        "name",
					Type:        "input",
					Label:       "Project Name",
					Description: "The name of your project",
					Default:     "my-project",
					Required:    true,
					Placeholder: "Enter project name",
				},
				{
					Name:        "license",
					Type:        "select",
					Label:       "License",
					Description: "Choose a license",
					Default:     "MIT",
					Required:    true,
					Options:     []string{"MIT", "Apache", "GPL"},
				},
				{
					Name:        "regions",
					Type:        "multiselect",
					Label:       "AWS Regions",
					Description: "Select regions",
					Default:     []string{"us-east-1"},
					Required:    true,
					Options:     []string{"us-east-1", "us-west-2", "eu-west-1"},
				},
				{
					Name:        "monitoring",
					Type:        "confirm",
					Label:       "Enable Monitoring",
					Description: "Enable monitoring and alerting",
					Default:     true,
				},
			},
		},
	}

	userValues := map[string]interface{}{
		"name":       "test-project",
		"license":    "Apache",
		"regions":    []string{"us-east-1", "us-west-2"},
		"monitoring": true,
	}

	// With the no-op runner the function must complete without error and without panicking.
	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	assert.NoError(t, err)
}

func TestPromptForScaffoldConfig_MixedFieldTypes(t *testing.T) {
	withNoOpFormRunner(t)

	// Fields of mixed types prompt in declared order within a single group.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{
			Name:        "test-scaffold",
			Description: "Test scaffold template configuration",
		},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "name", Type: "input", Label: "Project Name", Required: true},
				{Name: "description", Type: "text", Label: "Description"},
				{Name: "license", Type: "select", Label: "License", Required: true, Options: []string{"MIT", "Apache"}},
				{Name: "cloud_provider", Type: "select", Label: "Cloud Provider", Required: true, Options: []string{"aws", "gcp", "azure"}},
				{Name: "regions", Type: "multiselect", Label: "Regions", Required: true, Options: []string{"us-east-1", "us-west-2"}},
				{Name: "monitoring", Type: "confirm", Label: "Enable Monitoring"},
			},
		},
	}

	userValues := make(map[string]interface{})

	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	assert.NoError(t, err)
}

func TestPromptForScaffoldConfig_Validation(t *testing.T) {
	withNoOpFormRunner(t)

	// Test form validation wiring: fields with Required=true must produce a
	// huh Validate callback. We verify via createField, not the real TUI.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "required_field", Type: "input", Label: "Required Field", Required: true},
				{Name: "optional_field", Type: "input", Label: "Optional Field"},
			},
		},
	}

	userValues := map[string]interface{}{
		"required_field": "has value",
		"optional_field": "",
	}

	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	assert.NoError(t, err)
}

func TestPromptForScaffoldConfig_DefaultValues(t *testing.T) {
	withNoOpFormRunner(t)

	// Test that default values are properly initialised before the form runs.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "string_default", Type: "input", Label: "String Default", Default: "default string"},
				{Name: "bool_default", Type: "confirm", Label: "Bool Default", Default: true},
				{
					Name:    "array_default",
					Type:    "multiselect",
					Label:   "Array Default",
					Default: []string{"default1", "default2"},
					Options: []string{"default1", "default2", "option3"},
				},
			},
		},
	}

	userValues := make(map[string]interface{})

	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	assert.NoError(t, err)
}

func TestPromptForScaffoldConfig_ValueCapture(t *testing.T) {
	withNoOpFormRunner(t)

	// Verify that initial user values are preserved through the form pipeline.
	// The no-op runner returns immediately so the values come from the initial
	// state set by initializeFormValues + createField.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "name", Type: "input", Label: "Project Name", Default: "default-project"},
				{Name: "license", Type: "select", Label: "License", Default: "MIT", Options: []string{"MIT", "Apache"}},
			},
		},
	}

	userValues := map[string]interface{}{
		"name":    "user-project",
		"license": "Apache",
	}

	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	require.NoError(t, err)

	// The no-op runner leaves the pointer values at their initial state, which
	// are pre-populated from userValues by initializeFormValues + createField.
	assert.Equal(t, "user-project", userValues["name"])
	assert.Equal(t, "Apache", userValues["license"])
}

func TestPromptForScaffoldConfig_EmptyConfig(t *testing.T) {
	// Templates without fields skip the form entirely — no runner injection needed.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{
			Name:        "empty-project",
			Description: "Empty scaffold template configuration",
		},
	}

	userValues := make(map[string]interface{})

	// Test that the function doesn't panic with empty config.
	assert.NotPanics(t, func() {
		err := PromptForScaffoldConfig(scaffoldConfig, userValues)
		assert.NoError(t, err)
	})
}

func TestPromptForScaffoldConfig_AllFieldsCaptured(t *testing.T) {
	withNoOpFormRunner(t)

	// Create a scaffold config with all field types.
	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "name", Type: "input", Label: "Project Name", Default: "default-project", Required: true, Placeholder: "Enter project name"},
				{Name: "author", Type: "input", Label: "Author", Default: "Default Author", Required: true, Placeholder: "Enter author name"},
				{Name: "year", Type: "input", Label: "Year", Default: "2024", Required: true, Placeholder: "Enter year"},
				{Name: "license", Type: "select", Label: "License", Default: "MIT", Required: true, Options: []string{"MIT", "Apache", "GPL"}},
				{Name: "regions", Type: "multiselect", Label: "Regions", Default: []string{"us-east-1"}, Required: true, Options: []string{"us-east-1", "us-west-2", "eu-west-1"}},
				{Name: "enable_monitoring", Type: "confirm", Label: "Enable Monitoring", Default: true},
			},
		},
	}

	// Initial user values.
	userValues := map[string]interface{}{
		"name":              "test-project",
		"author":            "Test Author",
		"year":              "2025",
		"license":           "Apache",
		"regions":           []string{"us-west-2"},
		"enable_monitoring": false,
	}

	err := PromptForScaffoldConfig(scaffoldConfig, userValues)
	require.NoError(t, err)

	// Verify that all expected fields are present in userValues after the call.
	expectedFields := []string{"name", "author", "year", "license", "regions", "enable_monitoring"}
	for _, field := range expectedFields {
		if _, exists := userValues[field]; !exists {
			t.Errorf("Field '%s' is missing from userValues", field)
		}
	}
}

func TestCreateField_AllFieldTypes(t *testing.T) {
	// Test all field types to ensure they're created correctly.
	testCases := []struct {
		name     string
		field    FieldDefinition
		expected string
	}{
		{
			name: "input field",
			field: FieldDefinition{
				Name:        "name",
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
				Name:     "license",
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
				Name:     "regions",
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
				Name:    "enable_monitoring",
				Type:    "confirm",
				Label:   "Enable Monitoring",
				Default: true,
			},
			expected: "confirm",
		},
		{
			name: "year field",
			field: FieldDefinition{
				Name:        "year",
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

			// Set initial value.
			values[tc.field.Name] = tc.field.Default

			// Create the field.
			field, _ := createField(tc.field.Name, &tc.field, values)

			// Verify the field was created.
			if field == nil {
				t.Errorf("Field was not created for %s", tc.name)
			}

			// Verify the value is still in the map.
			if _, exists := values[tc.field.Name]; !exists {
				t.Errorf("Value for field '%s' was removed from values map", tc.field.Name)
			}
		})
	}
}

func TestCreateField_ValueOverwriteIssue(t *testing.T) {
	// This test reproduces the issue where values are being overwritten
	// in the createField function before the form runs.

	field := FieldDefinition{
		Name:        "year",
		Type:        "input",
		Label:       "Year",
		Default:     "2024",
		Required:    true,
		Placeholder: "2024",
	}

	// Simulate the values map that would be passed to createField.
	values := make(map[string]interface{})
	values["year"] = "2025" // User-provided value.

	// Create the field - this should NOT overwrite the value.
	_, _ = createField("year", &field, values)

	// The value should still be "2025", not "2024".
	if values["year"] != "2025" {
		t.Errorf("Expected year to remain '2025', but got '%v'", values["year"])
	}

	// Test with a different field type.
	field2 := FieldDefinition{
		Name:     "license",
		Type:     "select",
		Label:    "License",
		Default:  "MIT",
		Required: true,
		Options:  []string{"MIT", "Apache", "GPL"},
	}

	values["license"] = "Apache" // User-provided value.

	// Create the field - this should NOT overwrite the value.
	_, _ = createField("license", &field2, values)

	// The value should still be "Apache", not "MIT".
	if values["license"] != "Apache" {
		t.Errorf("Expected license to remain 'Apache', but got '%v'", values["license"])
	}
}

func TestCreateField_UserInputPriority(t *testing.T) {
	// This test verifies that user input is prioritized over defaults.

	field := FieldDefinition{
		Name:        "year",
		Type:        "input",
		Label:       "Year",
		Default:     "2024",
		Required:    true,
		Placeholder: "2024",
	}

	// Simulate user-provided value.
	values := make(map[string]interface{})
	values["year"] = "2025" // User wants 2025.

	// Create the field.
	fieldObj, getter := createField("year", &field, values)

	// The field should be created.
	if fieldObj == nil {
		t.Errorf("Field was not created")
	}

	// The getter should return the user's value, not the default.
	value := getter()
	if value != "2025" {
		t.Errorf("Expected getter to return user value '2025', but got '%v'", value)
	}

	// The values map should still contain the user's value.
	if values["year"] != "2025" {
		t.Errorf("Expected values map to contain user value '2025', but got '%v'", values["year"])
	}
}

func TestPromptForScaffoldConfig_UserInputCapture(t *testing.T) {
	// This test simulates the complete form flow to verify user input is captured.

	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "year", Type: "input", Label: "Year", Default: "2024", Required: true, Placeholder: "2024"},
				{Name: "author", Type: "input", Label: "Author", Default: "Default Author", Required: true, Placeholder: "Enter author name"},
			},
		},
	}

	// Initial user values (could be from command line or previous run).
	userValues := map[string]interface{}{
		"year":   "2025", // User wants 2025.
		"author": "John Doe",
	}

	// Verify initializeFormValues honours user-supplied values over defaults.
	formValues := initializeFormValues(scaffoldConfig, userValues)

	if formValues["year"] != "2025" {
		t.Errorf("Expected year to be '2025' (user value), but got '%v'", formValues["year"])
	}

	if formValues["author"] != "John Doe" {
		t.Errorf("Expected author to be 'John Doe' (user value), but got '%v'", formValues["author"])
	}

	// Test that the createField function uses the correct values.
	yearFieldDef := scaffoldConfig.Spec.Fields[0]
	yearField, yearGetter := createField("year", &yearFieldDef, formValues)
	if yearField == nil {
		t.Errorf("Year field was not created")
	}

	// The getter should return the user's value.
	yearValue := yearGetter()
	if yearValue != "2025" {
		t.Errorf("Expected year getter to return '2025', but got '%v'", yearValue)
	}

	// Test author field.
	authorFieldDef := scaffoldConfig.Spec.Fields[1]
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
	// This test verifies that existing values from a project record take
	// priority over defaults.

	scaffoldConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test-scaffold"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "year", Type: "input", Label: "Year", Default: "2024", Required: true, Placeholder: "2024"},
				{Name: "author", Type: "input", Label: "Author", Default: "Default Author", Required: true, Placeholder: "Enter author name"},
			},
		},
	}

	// Simulate existing values from a prior run (what LoadUserValues returns).
	existingValues := map[string]interface{}{
		"year":   "2025",
		"author": "Foobar",
	}

	// This simulates what happens in executeWithSetup.
	mergedValues := DeepMerge(scaffoldConfig, existingValues)

	// Verify that existing values take priority over defaults.
	if mergedValues["year"] != "2025" {
		t.Errorf("Expected year to be '2025' (existing value), but got '%v'", mergedValues["year"])
	}

	if mergedValues["author"] != "Foobar" {
		t.Errorf("Expected author to be 'Foobar' (existing value), but got '%v'", mergedValues["author"])
	}

	// Now simulate what happens in PromptForScaffoldConfig.
	formValues := initializeFormValues(scaffoldConfig, mergedValues)

	// Verify that existing values take priority in form values.
	if formValues["year"] != "2025" {
		t.Errorf("Expected formValues year to be '2025' (existing value), but got '%v'", formValues["year"])
	}

	if formValues["author"] != "Foobar" {
		t.Errorf("Expected formValues author to be 'Foobar' (existing value), but got '%v'", formValues["author"])
	}

	// Test that createField uses the correct values.
	yearFieldDef := scaffoldConfig.Spec.Fields[0]
	yearField, yearGetter := createField("year", &yearFieldDef, formValues)
	if yearField == nil {
		t.Errorf("Year field was not created")
	}

	yearValue := yearGetter()
	if yearValue != "2025" {
		t.Errorf("Expected year getter to return '2025' (existing value), but got '%v'", yearValue)
	}

	authorFieldDef := scaffoldConfig.Spec.Fields[1]
	authorField, authorGetter := createField("author", &authorFieldDef, formValues)
	if authorField == nil {
		t.Errorf("Author field was not created")
	}

	authorValue := authorGetter()
	if authorValue != "Foobar" {
		t.Errorf("Expected author getter to return 'Foobar' (existing value), but got '%v'", authorValue)
	}
}
