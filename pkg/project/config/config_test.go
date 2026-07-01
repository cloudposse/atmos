package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/manifest"
)

func TestLoadScaffoldConfigFromContent(t *testing.T) {
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: test-project
  description: Test project configuration
  version: 1.0.0
spec:
  fields:
    - name: project_name
      type: input
      label: Project Name
      description: The name of your project
      default: my-project
      required: true
      placeholder: Enter project name
    - name: license
      type: select
      label: License
      description: Choose a license
      default: MIT
      required: true
      options:
        - MIT
        - Apache
        - GPL`

	config, err := LoadScaffoldConfigFromContent(content)
	require.NoError(t, err)
	assert.Equal(t, manifest.DefaultAPIVersion, config.APIVersion)
	assert.Equal(t, ScaffoldKind, config.Kind)
	assert.Equal(t, "test-project", config.Metadata.Name)
	assert.Equal(t, "Test project configuration", config.Metadata.Description)
	assert.Equal(t, "1.0.0", config.Metadata.Version)
	require.Len(t, config.Spec.Fields, 2)

	// Field order is preserved as declared.
	projectNameField := config.Spec.Fields[0]
	assert.Equal(t, "project_name", projectNameField.Name)
	assert.Equal(t, "input", projectNameField.Type)
	assert.Equal(t, "Project Name", projectNameField.Label)
	assert.Equal(t, "my-project", projectNameField.Default)
	assert.True(t, projectNameField.Required)

	licenseField := config.Spec.Fields[1]
	assert.Equal(t, "license", licenseField.Name)
	assert.Equal(t, "select", licenseField.Type)
	assert.Equal(t, "License", licenseField.Label)
	assert.Equal(t, "MIT", licenseField.Default)
	assert.True(t, licenseField.Required)
	assert.Len(t, licenseField.Options, 3)
}

func TestLoadScaffoldConfigFromContent_InvalidManifests(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name: "legacy prompts format rejected",
			content: `name: legacy
prompts:
  - name: project_name
    type: input`,
			// Legacy documents declare no kind, which reads as a mismatch.
			wantErr: errUtils.ErrManifestKindMismatch,
		},
		{
			name: "legacy fields map format rejected",
			content: `name: legacy
fields:
  project_name:
    key: project_name
    type: input`,
			wantErr: errUtils.ErrManifestKindMismatch,
		},
		{
			name: "missing metadata name",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  description: no name here`,
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name: "wrong kind",
			content: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: x`,
			wantErr: errUtils.ErrManifestKindMismatch,
		},
		{
			name: "invalid field type",
			content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: x
spec:
  fields:
    - name: f
      type: dropdown`,
			wantErr: errUtils.ErrManifestValidation,
		},
		{
			name:    "invalid yaml",
			content: "kind: [unclosed",
			wantErr: errUtils.ErrManifestParse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadScaffoldConfigFromContent(tt.content)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestLoadScaffoldConfigFromFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "scaffold.yaml")
	content := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: from-file
spec:
  fields:
    - name: project_name
      type: input
      default: my-project
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))

	config, err := LoadScaffoldConfigFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "from-file", config.Metadata.Name)
	require.Len(t, config.Spec.Fields, 1)
	assert.Equal(t, "project_name", config.Spec.Fields[0].Name)

	// Missing file errors.
	_, err = LoadScaffoldConfigFromFile(filepath.Join(tempDir, "missing.yaml"))
	require.Error(t, err)
}

func TestLoadUserValues(t *testing.T) {
	tempDir := t.TempDir()

	// Loading from a directory without a record returns an empty map.
	values, err := LoadUserValues(tempDir)
	require.NoError(t, err)
	assert.Empty(t, values)

	// Write a project record and read the values back.
	template := &ScaffoldConfig{
		APIVersion: manifest.DefaultAPIVersion,
		Kind:       ScaffoldKind,
		Metadata:   manifest.Metadata{Name: "test-template"},
	}
	err = SaveProjectRecord(tempDir, template, SourceEmbedded, "", map[string]interface{}{
		"project_name": "test-project",
		"author":       "Test User",
		"license":      "MIT",
	})
	require.NoError(t, err)

	values, err = LoadUserValues(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "test-project", values["project_name"])
	assert.Equal(t, "Test User", values["author"])
	assert.Equal(t, "MIT", values["license"])
}

func TestLoadUserValues_HandWrittenRecord(t *testing.T) {
	tempDir := t.TempDir()

	// A record is a regular AtmosScaffoldConfig manifest on disk.
	recordContent := `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: rich-project
spec:
  values:
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
    terraform_version: 1.5.0
`
	atmosDir := filepath.Join(tempDir, ScaffoldConfigDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, ScaffoldConfigFileName), []byte(recordContent), 0o644))

	values, err := LoadUserValues(tempDir)
	require.NoError(t, err)

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

func TestLoadUserValues_LegacyRecordRejected(t *testing.T) {
	tempDir := t.TempDir()

	// Pre-manifest records (template_id/values without an envelope) are not
	// valid AtmosScaffoldConfig manifests.
	legacyContent := `template_id: test-template
values:
  project_name: test-project
`
	atmosDir := filepath.Join(tempDir, ScaffoldConfigDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, ScaffoldConfigFileName), []byte(legacyContent), 0o644))

	_, err := LoadUserValues(tempDir)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestValidation)
}

func TestDeepMerge(t *testing.T) {
	projectConfig := &ScaffoldConfig{
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "project_name", Default: "default-project"},
				{Name: "author", Default: "Default Author"},
				{Name: "license", Default: "MIT"},
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name": "user-project",
		"author":       "User Author",
	}

	merged := DeepMerge(projectConfig, userValues)

	// User values should override defaults.
	assert.Equal(t, "user-project", merged["project_name"])
	assert.Equal(t, "User Author", merged["author"])

	// Defaults should be preserved for missing values.
	assert.Equal(t, "MIT", merged["license"])
}

func TestDeepMerge_TemplateValuesOverrideDefaults(t *testing.T) {
	projectConfig := &ScaffoldConfig{
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "project_name", Default: "default-project"},
				{Name: "region", Default: "us-east-1"},
			},
			Values: map[string]any{
				"region": "eu-west-1", // Template preset beats the field default.
			},
		},
	}

	merged := DeepMerge(projectConfig, map[string]interface{}{
		"project_name": "user-project", // User value beats everything.
	})

	assert.Equal(t, "user-project", merged["project_name"])
	assert.Equal(t, "eu-west-1", merged["region"])
}

func TestCreateField(t *testing.T) {
	values := make(map[string]interface{})

	tests := []struct {
		name  string
		field FieldDefinition
	}{
		{
			name: "input field",
			field: FieldDefinition{
				Name:        "project_name",
				Type:        "input",
				Label:       "Project Name",
				Description: "The name of your project",
				Default:     "my-project",
				Required:    true,
				Placeholder: "Enter project name",
			},
		},
		{
			name: "select field",
			field: FieldDefinition{
				Name:        "license",
				Type:        "select",
				Label:       "License",
				Description: "Choose a license",
				Default:     "MIT",
				Required:    true,
				Options:     []string{"MIT", "Apache", "GPL"},
			},
		},
		{
			name: "multiselect field",
			field: FieldDefinition{
				Name:        "regions",
				Type:        "multiselect",
				Label:       "AWS Regions",
				Description: "Select regions",
				Default:     []string{"us-east-1"},
				Required:    true,
				Options:     []string{"us-east-1", "us-west-2", "eu-west-1"},
			},
		},
		{
			name: "confirm field",
			field: FieldDefinition{
				Name:        "monitoring",
				Type:        "confirm",
				Label:       "Enable Monitoring",
				Description: "Enable monitoring and alerting",
				Default:     true,
			},
		},
		{
			name: "unknown type falls back to input without panicking",
			field: FieldDefinition{
				Name: "mystery",
				Type: "dropdown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, getter := createField(tt.field.Name, &tt.field, values)
			assert.NotNil(t, field)
			assert.NotNil(t, getter)
		})
	}
}

func TestGetConfigurationSummary_OrderFollowsFields(t *testing.T) {
	projectConfig := &ScaffoldConfig{
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "zeta"},
				{Name: "alpha"},
				{Name: "mid"},
			},
		},
	}
	merged := map[string]interface{}{
		"zeta":  "1",
		"alpha": "2",
		"mid":   "3",
	}

	rows, header := GetConfigurationSummary(projectConfig, merged, map[string]string{"alpha": "flag"})

	assert.Equal(t, []string{"Setting", "Value", "Source"}, header)
	require.Len(t, rows, 3)
	// Rows follow declared field order, not map iteration order.
	assert.Equal(t, []string{"zeta", "1", "default"}, rows[0])
	assert.Equal(t, []string{"alpha", "2", "flag"}, rows[1])
	assert.Equal(t, []string{"mid", "3", "default"}, rows[2])
}

func TestPersistenceFlow(t *testing.T) {
	tempDir := t.TempDir()

	template := &ScaffoldConfig{
		APIVersion: manifest.DefaultAPIVersion,
		Kind:       ScaffoldKind,
		Metadata:   manifest.Metadata{Name: "rich-project", Version: "1.0.0"},
	}

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

	require.NoError(t, SaveProjectRecord(tempDir, template, SourceEmbedded, "abc123", cmdValues))

	recordPath := filepath.Join(tempDir, ScaffoldConfigDir, ScaffoldConfigFileName)
	assert.FileExists(t, recordPath)

	loadedValues, err := LoadUserValues(tempDir)
	require.NoError(t, err)

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

	// The record on disk is a readable AtmosScaffoldConfig manifest.
	content, err := os.ReadFile(recordPath)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "apiVersion: atmos/v1")
	assert.Contains(t, contentStr, "kind: AtmosScaffoldConfig")
	assert.Contains(t, contentStr, "name: rich-project")
	assert.Contains(t, contentStr, "source: embedded")
	assert.Contains(t, contentStr, "baseRef: abc123")
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

	projectConfig := &ScaffoldConfig{
		APIVersion: manifest.DefaultAPIVersion,
		Kind:       ScaffoldKind,
		Metadata:   manifest.Metadata{Name: "rich-project"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "project_name", Type: "input", Label: "Project Name", Default: "default-project", Required: true},
				{Name: "author", Type: "input", Label: "Author", Default: "Default Author", Required: true},
				{Name: "year", Type: "input", Label: "Year", Default: "2024", Required: true},
				{Name: "regions", Type: "multiselect", Label: "AWS Regions", Default: []string{"us-east-1"}, Required: true},
				{Name: "enable_monitoring", Type: "select", Label: "Enable Monitoring", Default: false, Required: true},
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name":      "my-custom-project",
		"author":            "Jane Smith",
		"year":              "2025",
		"regions":           []string{"us-west-2", "eu-west-1"},
		"enable_monitoring": true,
	}

	mergedValues := DeepMerge(projectConfig, userValues)

	assert.Equal(t, "my-custom-project", mergedValues["project_name"])
	assert.Equal(t, "Jane Smith", mergedValues["author"])
	assert.Equal(t, "2025", mergedValues["year"])
	assert.Equal(t, []string{"us-west-2", "eu-west-1"}, mergedValues["regions"])
	assert.Equal(t, true, mergedValues["enable_monitoring"])

	require.NoError(t, SaveProjectRecord(tempDir, projectConfig, "", "", mergedValues))

	// The questionnaire snapshot rides along in the record.
	record, err := LoadProjectRecord(tempDir)
	require.NoError(t, err)
	require.NotNil(t, record)
	require.Len(t, record.Spec.Fields, 5)
	assert.Equal(t, "project_name", record.Spec.Fields[0].Name)
	assert.Equal(t, "enable_monitoring", record.Spec.Fields[4].Name)

	loadedValues, err := LoadUserValues(tempDir)
	require.NoError(t, err)

	assert.Equal(t, "my-custom-project", loadedValues["project_name"])
	assert.Equal(t, "Jane Smith", loadedValues["author"])
	assert.Equal(t, "2025", loadedValues["year"])
	assert.Equal(t, []interface{}{"us-west-2", "eu-west-1"}, loadedValues["regions"])
	assert.Equal(t, true, loadedValues["enable_monitoring"])
}

func TestPromptForScaffoldConfig_NoFieldsIsNoop(t *testing.T) {
	// Templates without fields must not open a form at all, so this is safe
	// to run headless.
	projectConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "static"},
	}
	userValues := map[string]interface{}{}

	err := PromptForScaffoldConfig(projectConfig, userValues)
	assert.NoError(t, err)
	assert.Empty(t, userValues)
}

func TestPromptForScaffoldConfigUpdatesUserValues(t *testing.T) {
	// Skip this test in non-interactive environments.
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping interactive form test in non-interactive environment")
	}

	projectConfig := &ScaffoldConfig{
		Metadata: manifest.Metadata{Name: "test"},
		Spec: ScaffoldSpec{
			Fields: []FieldDefinition{
				{Name: "project_name", Type: "input", Label: "Project Name", Default: "default-project", Required: true, Placeholder: "Enter project name"},
				{Name: "author", Type: "input", Label: "Author", Default: "Default Author", Required: true, Placeholder: "Enter author name"},
				{Name: "year", Type: "input", Label: "Year", Default: "2024", Required: true, Placeholder: "Enter year"},
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name": "initial-project",
		"author":       "Initial Author",
		"year":         "2023",
	}

	// This test verifies that the function signature and structure are correct.
	// In a real interactive test, we would need to mock the form input.
	err := PromptForScaffoldConfig(projectConfig, userValues)

	// The function should either complete successfully or return an error
	// but it shouldn't crash.
	if err != nil && !strings.Contains(err.Error(), "user aborted") {
		t.Errorf("Unexpected error: %v", err)
	}
}
