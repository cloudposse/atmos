package scaffoldcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/internal/ui"
)

func TestLoadLocalTemplate_WithScaffoldConfig(t *testing.T) {
	// Create a temporary template directory with scaffold.yaml
	tempDir := t.TempDir()

	// Create scaffold.yaml
	scaffoldConfigContent := `name: "Test Template"
description: "A test template with scaffold configuration"
target_dir: "test-example"
template_id: "test-template"

fields:
  namespace:
    type: string
    label: "Namespace"
    default: "default"
  subdirectory:
    type: string
    label: "Subdirectory"
    default: ""
  enable_monitoring:
    type: confirm
    label: "Enable Monitoring"
    default: false`

	scaffoldConfigPath := filepath.Join(tempDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldConfigPath, []byte(scaffoldConfigContent), 0644); err != nil {
		t.Fatalf("Failed to create %s: %v", "scaffold.yaml", err)
	}

	// Create some template files
	files := map[string]string{
		"README.md":                         "Test template README",
		"{{.Config.namespace}}/config.yaml": "namespace: {{.Config.namespace}}",
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml":                     "deep config",
		"{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}": "monitoring config",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	// Test loading the template
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Verify the configuration
	if config.Name != "Test Template" {
		t.Errorf("Expected name 'Test Template', got '%s'", config.Name)
	}
	if config.Description != "A test template with scaffold configuration" {
		t.Errorf("Expected description 'A test template with scaffold configuration', got '%s'", config.Description)
	}

	// Verify files were loaded
	expectedFiles := []string{
		"README.md",
		"{{.Config.namespace}}/config.yaml",
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml",
		"{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}",
		"scaffold.yaml",
	}

	if len(config.Files) != len(expectedFiles) {
		t.Errorf("Expected %d files, got %d", len(expectedFiles), len(config.Files))
	}

	// Check that all expected files are present
	for _, expectedFile := range expectedFiles {
		found := false
		for _, file := range config.Files {
			if file.Path == expectedFile {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected file '%s' not found in configuration", expectedFile)
		}
	}
}

func TestLoadLocalTemplate_WithoutScaffoldConfig(t *testing.T) {
	// Create a temporary template directory without scaffold.yaml
	tempDir := t.TempDir()

	// Create some template files
	files := map[string]string{
		"README.md":                            "Test template README",
		"config.yaml":                          "basic config",
		"templates/{{.Config.namespace}}.yaml": "template with namespace",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	// Test loading the template
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Verify the configuration uses directory name
	expectedName := filepath.Base(tempDir)
	if config.Name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, config.Name)
	}
	if config.Description != "Template from local directory" {
		t.Errorf("Expected description 'Template from local directory', got '%s'", config.Description)
	}

	// Verify files were loaded
	expectedFiles := []string{
		"README.md",
		"config.yaml",
		"templates/{{.Config.namespace}}.yaml",
	}

	if len(config.Files) != len(expectedFiles) {
		t.Errorf("Expected %d files, got %d", len(expectedFiles), len(config.Files))
	}
}

func TestScaffoldFileSkipping_WithScaffoldConfig(t *testing.T) {
	// Create a temporary template directory with scaffold.yaml
	tempDir := t.TempDir()

	// Create scaffold.yaml
	scaffoldConfigContent := `name: "File Skipping Test"
description: "Test template for file skipping behavior"
target_dir: "file-skipping-test"
template_id: "file-skipping-test"

fields:
  namespace:
    type: string
    label: "Namespace"
    default: "default"
  subdirectory:
    type: string
    label: "Subdirectory"
    default: ""
  enable_monitoring:
    type: confirm
    label: "Enable Monitoring"
    default: false`

	scaffoldConfigPath := filepath.Join(tempDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldConfigPath, []byte(scaffoldConfigContent), 0644); err != nil {
		t.Fatalf("Failed to create %s: %v", "scaffold.yaml", err)
	}

	// Create template files with various path patterns
	files := map[string]string{
		"README.md":                         "Test template README",
		"{{.Config.namespace}}/config.yaml": "namespace: {{.Config.namespace}}",
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml":                     "deep config",
		"{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}": "monitoring config",
		"static/file.txt": "static content",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	// Load the template
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Test with default values (should skip files with empty subdirectory)
	targetDir := t.TempDir()
	ui := ui.NewInitUI()

	// Use default values (subdirectory is empty, enable_monitoring is false)
	cmdValues := map[string]interface{}{
		"namespace":         "default",
		"subdirectory":      "",
		"enable_monitoring": false,
	}

	err = ui.Execute(*config, targetDir, false, false, true, cmdValues)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// Check that files with empty path segments were skipped
	expectedCreated := []string{
		"README.md",
		"default/config.yaml",
		"static/file.txt",
	}

	expectedSkipped := []string{
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml",                     // Should be skipped due to empty subdirectory
		"{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}", // Should be skipped due to false condition
	}

	// Check created files
	for _, expectedFile := range expectedCreated {
		filePath := filepath.Join(targetDir, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
		}
	}

	// Check that skipped files don't exist
	for _, skippedFile := range expectedSkipped {
		// The skipped files should not exist in the target directory
		// We can't check the exact path since they were skipped, but we can verify
		// that no files with "deep.yaml" or "monitoring.yaml" exist
		if skippedFile == "{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml" {
			deepPath := filepath.Join(targetDir, "default", "deep.yaml")
			if _, err := os.Stat(deepPath); err == nil {
				t.Errorf("Expected file '%s' to be skipped, but it exists at %s", skippedFile, deepPath)
			}
		}
		if skippedFile == "{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}" {
			monitoringPath := filepath.Join(targetDir, "default", "monitoring.yaml")
			if _, err := os.Stat(monitoringPath); err == nil {
				t.Errorf("Expected file '%s' to be skipped, but it exists at %s", skippedFile, monitoringPath)
			}
		}
	}
}

func TestScaffoldFileSkipping_WithoutScaffoldConfig(t *testing.T) {
	// Create a temporary template directory without scaffold.yaml
	tempDir := t.TempDir()

	// Create template files with template variables
	files := map[string]string{
		"README.md":                         "Test template README",
		"{{.Config.namespace}}/config.yaml": "namespace: {{.Config.namespace}}",
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml": "deep config",
		"static/file.txt": "static content",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	// Load the template
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Test with command-line values
	targetDir := t.TempDir()
	ui := ui.NewInitUI()

	// Provide command-line values
	cmdValues := map[string]interface{}{
		"namespace":    "production",
		"subdirectory": "", // Empty subdirectory should cause skipping
	}

	err = ui.Execute(*config, targetDir, false, false, true, cmdValues)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// Check that files with empty path segments were skipped
	expectedCreated := []string{
		"README.md",
		"production/config.yaml",
		"static/file.txt",
	}

	// Check created files
	for _, expectedFile := range expectedCreated {
		filePath := filepath.Join(targetDir, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
		}
	}

	// Check that files with empty subdirectory were skipped
	deepPath := filepath.Join(targetDir, "production", "deep.yaml")
	if _, err := os.Stat(deepPath); err == nil {
		t.Errorf("Expected file with empty subdirectory to be skipped, but it exists at %s", deepPath)
	}
}

func TestScaffoldFileSkipping_WithValidSubdirectory(t *testing.T) {
	// Create a temporary template directory with scaffold.yaml
	tempDir := t.TempDir()

	// Create scaffold.yaml
	scaffoldConfigContent := `name: "Valid Subdirectory Test"
description: "Test template with valid subdirectory"
target_dir: "valid-subdirectory-test"
template_id: "valid-subdirectory-test"

fields:
  namespace:
    type: string
    label: "Namespace"
    default: "default"
  subdirectory:
    type: string
    label: "Subdirectory"
    default: "config"
  enable_monitoring:
    type: confirm
    label: "Enable Monitoring"
    default: true`

	scaffoldConfigPath := filepath.Join(tempDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldConfigPath, []byte(scaffoldConfigContent), 0644); err != nil {
		t.Fatalf("Failed to create %s: %v", "scaffold.yaml", err)
	}

	// Create template files
	files := map[string]string{
		"README.md":                         "Test template README",
		"{{.Config.namespace}}/config.yaml": "namespace: {{.Config.namespace}}",
		"{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml":                     "deep config",
		"{{if .Config.enable_monitoring}}{{.Config.namespace}}/monitoring.yaml{{end}}": "monitoring config",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}

	// Load the template
	config, err := loadLocalTemplate(tempDir)
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Test with valid values (should create all files)
	targetDir := t.TempDir()
	ui := ui.NewInitUI()

	// Use valid values
	cmdValues := map[string]interface{}{
		"namespace":         "production",
		"subdirectory":      "config",
		"enable_monitoring": true,
	}

	err = ui.Execute(*config, targetDir, false, false, true, cmdValues)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// Check that all files were created
	expectedCreated := []string{
		"README.md",
		"production/config.yaml",
		"production/config/deep.yaml",
		"production/monitoring.yaml",
	}

	for _, expectedFile := range expectedCreated {
		filePath := filepath.Join(targetDir, expectedFile)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file '%s' to be created, but it doesn't exist", expectedFile)
		}
	}
}

func TestListScaffoldTemplates_WithValidConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create atmos.yaml with scaffold configuration
	atmosConfig := `# Atmos CLI Configuration
base_path: .

scaffold:
  templates:
    test-template:
      source: "github.com/test/template"
      ref: "v1.0.0"
      description: "A test template for unit testing"
      target_dir: "./test"
      values:
        key: "value"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosConfig), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to the test directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	// Test listing scaffold templates
	err = listScaffoldTemplates()
	if err != nil {
		t.Fatalf("Expected no error when listing valid templates, got: %v", err)
	}
}

func TestListScaffoldTemplates_WithNoScaffoldSection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create atmos.yaml without scaffold section
	atmosConfig := `# Atmos CLI Configuration
base_path: .

components:
  terraform:
    base_path: "components/terraform"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosConfig), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to the test directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	// Test listing scaffold templates - should not return error
	err = listScaffoldTemplates()
	if err != nil {
		t.Fatalf("Expected no error when no scaffold section exists, got: %v", err)
	}
}

func TestListScaffoldTemplates_WithEmptyTemplates(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create atmos.yaml with empty templates section
	atmosConfig := `# Atmos CLI Configuration
base_path: .

scaffold:
  templates: {}`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosConfig), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to the test directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	// Test listing scaffold templates - should not return error
	err = listScaffoldTemplates()
	if err != nil {
		t.Fatalf("Expected no error when templates section is empty, got: %v", err)
	}
}

func TestListScaffoldTemplates_WithInvalidTemplatesSection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create atmos.yaml with invalid templates section
	atmosConfig := `# Atmos CLI Configuration
base_path: .

components:
  terraform:
    base_path: components/terraform

scaffold:
  templates: "not-a-map"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosConfig), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to the temporary directory
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	// Test listing scaffold templates - should return error
	err = listScaffoldTemplates()
	if err == nil {
		t.Fatal("Expected error when templates section is invalid, but got none")
	}

	expectedError := "templates section is not a valid configuration"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}

func TestReadScaffoldConfig_WithValidConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create atmos.yaml with scaffold configuration
	atmosConfig := `# Atmos CLI Configuration
base_path: .

scaffold:
  templates:
    test-template:
      source: "github.com/test/template"
      ref: "v1.0.0"
      target_dir: "./test"
      values:
        key: "value"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosConfig), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Test reading scaffold config
	scaffoldConfig, err := readScaffoldConfig(tempDir)
	if err != nil {
		t.Fatalf("Failed to read scaffold config: %v", err)
	}

	// Verify scaffold config structure
	templates, ok := scaffoldConfig["templates"]
	if !ok {
		t.Fatal("Expected 'templates' key in scaffold config")
	}

	templatesMap, ok := templates.(map[string]interface{})
	if !ok {
		t.Fatal("Expected templates to be a map")
	}

	// Verify template configuration
	templateConfig, ok := templatesMap["test-template"]
	if !ok {
		t.Fatal("Expected 'test-template' in templates")
	}

	templateMap, ok := templateConfig.(map[string]interface{})
	if !ok {
		t.Fatal("Expected template config to be a map")
	}

	// Verify template properties
	source, ok := templateMap["source"].(string)
	if !ok || source != "github.com/test/template" {
		t.Errorf("Expected source 'github.com/test/template', got '%s'", source)
	}

	ref, ok := templateMap["ref"].(string)
	if !ok || ref != "v1.0.0" {
		t.Errorf("Expected ref 'v1.0.0', got '%s'", ref)
	}

	targetDir, ok := templateMap["target_dir"].(string)
	if !ok || targetDir != "./test" {
		t.Errorf("Expected target_dir './test', got '%s'", targetDir)
	}
}

func TestReadScaffoldConfig_WithNoAtmosYaml(t *testing.T) {
	// Create a temporary directory without atmos.yaml
	tempDir := t.TempDir()

	// Test reading scaffold config - should return error
	_, err := readScaffoldConfig(tempDir)
	if err == nil {
		t.Fatal("Expected error when atmos.yaml doesn't exist, but got none")
	}

	expectedError := "failed to read atmos.yaml"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Expected error containing '%s', got '%s'", expectedError, err.Error())
	}
}
