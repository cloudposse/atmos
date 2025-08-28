package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/stretchr/testify/assert"
)

func TestLoadScaffoldConfigFromContent(t *testing.T) {
	content := `name: "Test Project"
description: "Test project configuration"
version: "1.0.0"
fields:
  project_name:
    key: "project_name"
    type: "input"
    label: "Project Name"
    description: "The name of your project"
    default: "my-project"
    required: true
    placeholder: "Enter project name"
  license:
    key: "license"
    type: "select"
    label: "License"
    description: "Choose a license"
    default: "MIT"
    required: true
    options:
      - "MIT"
      - "Apache"
      - "GPL"`

	config, err := LoadScaffoldConfigFromContent(content)
	assert.NoError(t, err)
	assert.Equal(t, "Test Project", config.Name)
	assert.Equal(t, "Test project configuration", config.Description)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Len(t, config.Fields, 2)

	// Check project_name field
	projectNameField, exists := config.Fields["project_name"]
	assert.True(t, exists)
	assert.Equal(t, "input", projectNameField.Type)
	assert.Equal(t, "Project Name", projectNameField.Label)
	assert.Equal(t, "my-project", projectNameField.Default)
	assert.True(t, projectNameField.Required)

	// Check license field
	licenseField, exists := config.Fields["license"]
	assert.True(t, exists)
	assert.Equal(t, "select", licenseField.Type)
	assert.Equal(t, "License", licenseField.Label)
	assert.Equal(t, "MIT", licenseField.Default)
	assert.True(t, licenseField.Required)
	assert.Len(t, licenseField.Options, 3)
}

func TestLoadUserValues(t *testing.T) {
	tempDir := t.TempDir()

	// Test loading from non-existent file
	values, err := LoadUserValues(tempDir)
	assert.NoError(t, err)
	assert.Empty(t, values)

	// Test loading from existing file
	configDir := filepath.Join(tempDir, ".atmos")
	err = os.MkdirAll(configDir, 0755)
	assert.NoError(t, err)

	configPath := filepath.Join(configDir, "scaffold.yaml")
	configContent := `template_id: "test-template"
values:
  project_name: "test-project"
  author: "Test User"
  license: "MIT"`

	err = os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err)

	values, err = LoadUserValues(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, "test-project", values["project_name"])
	assert.Equal(t, "Test User", values["author"])
	assert.Equal(t, "MIT", values["license"])
}

func TestLoadUserValues_ExistingConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock config.yaml with existing values
	configContent := `values:
  author: Foobar
  year: "2025"
  license: Apache Software License 2.0
  cloud_provider: aws
  enable_logging: true
  enable_monitoring: true
  environment: dev
  project_description: An Atmos project for managing infrastructure as code
  project_name: my-atmos-project
  regions:
    - us-west-2
    - eu-west-1
  terraform_version: 1.5.0`

	// Create the .atmos directory and config.yaml
	atmosDir := filepath.Join(tempDir, ".atmos")
	err := os.MkdirAll(atmosDir, 0755)
	assert.NoError(t, err)

	configPath := filepath.Join(atmosDir, "scaffold.yaml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err)

	// Load the values
	values, err := LoadUserValues(tempDir)
	assert.NoError(t, err)

	// Verify all values are loaded correctly
	assert.Equal(t, "Foobar", values["author"])
	assert.Equal(t, "2025", values["year"])
	assert.Equal(t, "Apache Software License 2.0", values["license"])
	assert.Equal(t, "aws", values["cloud_provider"])
	assert.Equal(t, true, values["enable_logging"])
	assert.Equal(t, true, values["enable_monitoring"])
	assert.Equal(t, "dev", values["environment"])
	assert.Equal(t, "An Atmos project for managing infrastructure as code", values["project_description"])
	assert.Equal(t, "my-atmos-project", values["project_name"])
	assert.Equal(t, []interface{}{"us-west-2", "eu-west-1"}, values["regions"])
	assert.Equal(t, "1.5.0", values["terraform_version"])
}

func TestSaveUserValues(t *testing.T) {
	tempDir := t.TempDir()

	values := map[string]interface{}{
		"project_name": "test-project",
		"author":       "Test User",
		"license":      "MIT",
		"regions":      []string{"us-east-1", "us-west-2"},
		"monitoring":   true,
	}

	err := SaveUserValues(tempDir, values)
	assert.NoError(t, err)

	// Verify file was created
	configPath := filepath.Join(tempDir, ".atmos", "scaffold.yaml")
	assert.FileExists(t, configPath)

	// Load and verify content
	loadedValues, err := LoadUserValues(tempDir)
	assert.NoError(t, err)
	assert.Equal(t, "test-project", loadedValues["project_name"])
	assert.Equal(t, "Test User", loadedValues["author"])
	assert.Equal(t, "MIT", loadedValues["license"])
	assert.Equal(t, []interface{}{"us-east-1", "us-west-2"}, loadedValues["regions"])
	assert.Equal(t, true, loadedValues["monitoring"])
}

func TestDeepMerge(t *testing.T) {
	projectConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"project_name": {
				Default: "default-project",
			},
			"author": {
				Default: "Default Author",
			},
			"license": {
				Default: "MIT",
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name": "user-project",
		"author":       "User Author",
	}

	merged := DeepMerge(projectConfig, userValues)

	// User values should override defaults
	assert.Equal(t, "user-project", merged["project_name"])
	assert.Equal(t, "User Author", merged["author"])

	// Defaults should be preserved for missing values
	assert.Equal(t, "MIT", merged["license"])
}

func TestCreateField(t *testing.T) {
	values := make(map[string]interface{})

	// Test input field
	inputField := FieldDefinition{
		Key:         "project_name",
		Type:        "input",
		Label:       "Project Name",
		Description: "The name of your project",
		Default:     "my-project",
		Required:    true,
		Placeholder: "Enter project name",
	}

	field, _ := createField("project_name", inputField, values)
	assert.NotNil(t, field)

	// Test select field
	selectField := FieldDefinition{
		Key:         "license",
		Type:        "select",
		Label:       "License",
		Description: "Choose a license",
		Default:     "MIT",
		Required:    true,
		Options:     []string{"MIT", "Apache", "GPL"},
	}

	field, _ = createField("license", selectField, values)
	assert.NotNil(t, field)

	// Test multiselect field
	multiSelectField := FieldDefinition{
		Key:         "regions",
		Type:        "multiselect",
		Label:       "AWS Regions",
		Description: "Select regions",
		Default:     []string{"us-east-1"},
		Required:    true,
		Options:     []string{"us-east-1", "us-west-2", "eu-west-1"},
	}

	field, _ = createField("regions", multiSelectField, values)
	assert.NotNil(t, field)

	// Test confirm field
	confirmField := FieldDefinition{
		Key:         "monitoring",
		Type:        "confirm",
		Label:       "Enable Monitoring",
		Description: "Enable monitoring and alerting",
		Default:     true,
		Required:    false,
	}

	field, _ = createField("monitoring", confirmField, values)
	assert.NotNil(t, field)
}

func TestPersistenceFlow(t *testing.T) {
	tempDir := t.TempDir()

	// Test data that matches what we expect from command-line
	cmdValues := map[string]interface{}{
		"project_name":        "my-test-project",
		"project_description": "An Atmos project for managing infrastructure as code",
		"author":              "John Doe",
		"year":                "2024",
		"license":             "MIT",
		"cloud_provider":      "aws",
		"environment":         "dev",
		"terraform_version":   "1.5.0",
		"regions":             []string{"us-east-1", "us-west-2"},
		"enable_monitoring":   true,
		"enable_logging":      true,
	}

	// Save the values
	err := SaveUserValues(tempDir, cmdValues)
	assert.NoError(t, err)

	// Verify file was created
	configPath := filepath.Join(tempDir, ".atmos", "scaffold.yaml")
	assert.FileExists(t, configPath)

	// Load and verify all values are persisted correctly
	loadedValues, err := LoadUserValues(tempDir)
	assert.NoError(t, err)

	// Test all the key values
	assert.Equal(t, "my-test-project", loadedValues["project_name"])
	assert.Equal(t, "An Atmos project for managing infrastructure as code", loadedValues["project_description"])
	assert.Equal(t, "John Doe", loadedValues["author"])
	assert.Equal(t, "2024", loadedValues["year"])
	assert.Equal(t, "MIT", loadedValues["license"])
	assert.Equal(t, "aws", loadedValues["cloud_provider"])
	assert.Equal(t, "dev", loadedValues["environment"])
	assert.Equal(t, "1.5.0", loadedValues["terraform_version"])
	assert.Equal(t, []interface{}{"us-east-1", "us-west-2"}, loadedValues["regions"])
	assert.Equal(t, true, loadedValues["enable_monitoring"])
	assert.Equal(t, true, loadedValues["enable_logging"])

	// Test that the file content is actually readable
	content, err := os.ReadFile(configPath)
	assert.NoError(t, err)

	// Verify key content is in the file
	contentStr := string(content)
	assert.Contains(t, contentStr, "project_name: my-test-project")
	assert.Contains(t, contentStr, "author: John Doe")
	assert.Contains(t, contentStr, "year: \"2024\"")
	assert.Contains(t, contentStr, "regions:")
	assert.Contains(t, contentStr, "- us-east-1")
	assert.Contains(t, contentStr, "- us-west-2")
	assert.Contains(t, contentStr, "enable_monitoring: true")
	assert.Contains(t, contentStr, "enable_logging: true")
}

func TestPersistenceWithScaffoldConfig(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock project config
	projectConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"project_name": {
				Key:      "project_name",
				Type:     "input",
				Label:    "Project Name",
				Default:  "default-project",
				Required: true,
			},
			"author": {
				Key:      "author",
				Type:     "input",
				Label:    "Author",
				Default:  "Default Author",
				Required: true,
			},
			"year": {
				Key:      "year",
				Type:     "input",
				Label:    "Year",
				Default:  "2024",
				Required: true,
			},
			"regions": {
				Key:      "regions",
				Type:     "multiselect",
				Label:    "AWS Regions",
				Default:  []string{"us-east-1"},
				Required: true,
			},
			"enable_monitoring": {
				Key:      "enable_monitoring",
				Type:     "select",
				Label:    "Enable Monitoring",
				Default:  false,
				Required: true,
			},
		},
	}

	// Test user values that override defaults
	userValues := map[string]interface{}{
		"project_name":      "my-custom-project",
		"author":            "Jane Smith",
		"year":              "2025",
		"regions":           []string{"us-west-2", "eu-west-1"},
		"enable_monitoring": true,
	}

	// Test the deep merge functionality
	mergedValues := DeepMerge(projectConfig, userValues)

	// Verify merged values
	assert.Equal(t, "my-custom-project", mergedValues["project_name"])
	assert.Equal(t, "Jane Smith", mergedValues["author"])
	assert.Equal(t, "2025", mergedValues["year"])
	assert.Equal(t, []string{"us-west-2", "eu-west-1"}, mergedValues["regions"])
	assert.Equal(t, true, mergedValues["enable_monitoring"])

	// Save the merged values
	err := SaveUserValues(tempDir, mergedValues)
	assert.NoError(t, err)

	// Load and verify persistence
	loadedValues, err := LoadUserValues(tempDir)
	assert.NoError(t, err)

	assert.Equal(t, "my-custom-project", loadedValues["project_name"])
	assert.Equal(t, "Jane Smith", loadedValues["author"])
	assert.Equal(t, "2025", loadedValues["year"])
	assert.Equal(t, []interface{}{"us-west-2", "eu-west-1"}, loadedValues["regions"])
	assert.Equal(t, true, loadedValues["enable_monitoring"])
}

func TestPromptForScaffoldConfigUpdatesUserValues(t *testing.T) {
	// Skip this test in non-interactive environments
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	// Create a simple project config
	projectConfig := &ScaffoldConfig{
		Fields: map[string]FieldDefinition{
			"project_name": {
				Key:         "project_name",
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
		},
	}

	// Initial user values
	userValues := map[string]interface{}{
		"project_name": "initial-project",
		"author":       "Initial Author",
		"year":         "2023",
	}

	// Mock the form input by setting environment variables or using a test approach
	// For now, we'll test that the function doesn't crash and handles the userValues correctly

	// This test verifies that the function signature and structure are correct
	// In a real interactive test, we would need to mock the form input
	err := PromptForScaffoldConfig(projectConfig, userValues)

	// The function should either complete successfully or return an error
	// but it shouldn't crash
	if err != nil && !strings.Contains(err.Error(), "user aborted") {
		t.Errorf("Unexpected error: %v", err)
	}
}
