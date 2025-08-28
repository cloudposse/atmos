package scaffoldcmd

import (
	"os"
	"path/filepath"
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
	config, err := loadLocalTemplate(tempDir, "test-template")
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
	config, err := loadLocalTemplate(tempDir, "test-template")
	if err != nil {
		t.Fatalf("Failed to load template: %v", err)
	}

	// Verify the configuration uses template key name
	expectedName := "test-template"
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
	config, err := loadLocalTemplate(tempDir, "test-template")
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
	config, err := loadLocalTemplate(tempDir, "test-template")
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
	config, err := loadLocalTemplate(tempDir, "test-template")
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

func TestGenerateProject_ScaffoldTemplate(t *testing.T) {
	// Create a temporary atmos.yaml with scaffold configuration
	tempDir := t.TempDir()
	atmosYAML := `scaffold:
  templates:
    test-template:
      source: "./test-template"
      ref: "local"
      target_dir: "./{{ .Config.name }}"
      description: "Test template"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosYAML), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Create test template directory
	templateDir := filepath.Join(tempDir, "test-template")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template directory: %v", err)
	}

	// Create scaffold.yaml in template
	scaffoldConfig := `name: "Test Template"
description: "A test template"
template_id: "test-template"

fields:
  name:
    type: string
    label: "Name"
    default: "test-project"`

	scaffoldPath := filepath.Join(templateDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldPath, []byte(scaffoldConfig), 0644); err != nil {
		t.Fatalf("Failed to create scaffold.yaml: %v", err)
	}

	// Create a simple template file
	templateFile := filepath.Join(templateDir, "README.md")
	if err := os.WriteFile(templateFile, []byte("# {{ .Config.name }}"), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test generateProject with scaffold template
	targetPath := filepath.Join(tempDir, "output")
	err = generateProject("test-template", targetPath, false, false, true, 50, map[string]interface{}{
		"name": "my-test-project",
	})
	if err != nil {
		t.Fatalf("Failed to generate project: %v", err)
	}

	// Verify output was created
	outputFile := filepath.Join(targetPath, "README.md")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected README.md to be created")
	}
}

func TestGenerateProject_LocalTemplate(t *testing.T) {
	// Create a temporary template directory
	tempDir := t.TempDir()
	templateDir := filepath.Join(tempDir, "local-template")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template directory: %v", err)
	}

	// Create a simple template file
	templateFile := filepath.Join(templateDir, "README.md")
	if err := os.WriteFile(templateFile, []byte("# Local Template"), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Test generateProject with local template
	targetPath := filepath.Join(tempDir, "output")
	err := generateProject(templateDir, targetPath, false, false, true, 50, map[string]interface{}{
		"name": "test-project",
	})
	if err != nil {
		t.Fatalf("Failed to generate project: %v", err)
	}

	// Verify output was created
	outputFile := filepath.Join(targetPath, "README.md")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected README.md to be created")
	}
}

func TestGenerateProject_RemoteTemplate(t *testing.T) {
	// Test generateProject with remote template (should fail gracefully)
	targetPath := filepath.Join(t.TempDir(), "output")
	err := generateProject("https://github.com/nonexistent/template.git", targetPath, false, false, true, 50, make(map[string]interface{}))
	if err == nil {
		t.Error("Expected error for non-existent remote template")
	}
}

func TestIsScaffoldTemplate(t *testing.T) {
	// Create a temporary atmos.yaml with scaffold configuration
	tempDir := t.TempDir()
	atmosYAML := `scaffold:
  templates:
    test-template:
      source: "./test-template"
      ref: "local"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosYAML), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test valid scaffold template
	if !isScaffoldTemplate("test-template") {
		t.Error("Expected 'test-template' to be recognized as scaffold template")
	}

	// Test invalid scaffold template
	if isScaffoldTemplate("nonexistent-template") {
		t.Error("Expected 'nonexistent-template' to not be recognized as scaffold template")
	}
}

func TestIsRemoteTemplate(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "https URL",
			path:     "https://github.com/user/template.git",
			expected: true,
		},
		{
			name:     "http URL",
			path:     "http://github.com/user/template.git",
			expected: true,
		},
		{
			name:     "git URL",
			path:     "git://github.com/user/template.git",
			expected: true,
		},
		{
			name:     "ssh URL",
			path:     "ssh://git@github.com/user/template.git",
			expected: true,
		},
		{
			name:     "local path",
			path:     "./local-template",
			expected: false,
		},
		{
			name:     "absolute path",
			path:     "/tmp/template",
			expected: false,
		},
		{
			name:     "relative path",
			path:     "template",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRemoteTemplate(tc.path)
			if result != tc.expected {
				t.Errorf("Expected isRemoteTemplate('%s') to be %v, got %v", tc.path, tc.expected, result)
			}
		})
	}
}

func TestGenerateFromScaffoldTemplate_ValidTemplate(t *testing.T) {
	// Create a temporary atmos.yaml with scaffold configuration
	tempDir := t.TempDir()
	atmosYAML := `scaffold:
  templates:
    test-template:
      source: "./test-template"
      ref: "local"
      target_dir: "./{{ .Config.name }}"
      description: "Test template"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosYAML), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Create test template directory
	templateDir := filepath.Join(tempDir, "test-template")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template directory: %v", err)
	}

	// Create scaffold.yaml in template
	scaffoldConfig := `name: "Test Template"
description: "A test template"
template_id: "test-template"

fields:
  name:
    type: string
    label: "Name"
    default: "test-project"`

	scaffoldPath := filepath.Join(templateDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldPath, []byte(scaffoldConfig), 0644); err != nil {
		t.Fatalf("Failed to create scaffold.yaml: %v", err)
	}

	// Create a simple template file
	templateFile := filepath.Join(templateDir, "README.md")
	if err := os.WriteFile(templateFile, []byte("# {{ .Config.name }}"), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test generateFromScaffoldTemplate
	ui := ui.NewInitUI()
	targetPath := filepath.Join(tempDir, "output")
	err = generateFromScaffoldTemplate("test-template", targetPath, false, false, true, map[string]interface{}{
		"name": "my-test-project",
	}, ui)
	if err != nil {
		t.Fatalf("Failed to generate from scaffold template: %v", err)
	}

	// Verify output was created
	outputFile := filepath.Join(targetPath, "README.md")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected README.md to be created")
	}
}

func TestGenerateFromScaffoldTemplate_InvalidTemplate(t *testing.T) {
	// Create a temporary atmos.yaml with scaffold configuration
	tempDir := t.TempDir()
	atmosYAML := `scaffold:
  templates:
    test-template:
      source: "./test-template"
      ref: "local"`

	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if err := os.WriteFile(atmosPath, []byte(atmosYAML), 0644); err != nil {
		t.Fatalf("Failed to create atmos.yaml: %v", err)
	}

	// Change to temp directory for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	// Test generateFromScaffoldTemplate with invalid template
	ui := ui.NewInitUI()
	targetPath := filepath.Join(tempDir, "output")
	err = generateFromScaffoldTemplate("nonexistent-template", targetPath, false, false, true, make(map[string]interface{}), ui)
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}

func TestValidateScaffoldTemplate_Valid(t *testing.T) {
	tempDir := t.TempDir()

	// Create .atmos directory
	atmosDir := filepath.Join(tempDir, ".atmos")
	if err := os.MkdirAll(atmosDir, 0755); err != nil {
		t.Fatalf("Failed to create .atmos directory: %v", err)
	}

	// Create scaffold.yaml with valid template
	scaffoldConfig := `template: "test-template"
values:
  name: "test-project"`

	scaffoldPath := filepath.Join(atmosDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldPath, []byte(scaffoldConfig), 0644); err != nil {
		t.Fatalf("Failed to create scaffold.yaml: %v", err)
	}

	// Test validation
	err := validateScaffoldTemplate(tempDir, "test-template")
	if err != nil {
		t.Errorf("Expected validation to pass, got error: %v", err)
	}
}

func TestValidateScaffoldTemplate_Mismatch(t *testing.T) {
	tempDir := t.TempDir()

	// Create .atmos directory
	atmosDir := filepath.Join(tempDir, ".atmos")
	if err := os.MkdirAll(atmosDir, 0755); err != nil {
		t.Fatalf("Failed to create .atmos directory: %v", err)
	}

	// Create scaffold.yaml with different template
	scaffoldConfig := `template: "different-template"
values:
  name: "test-project"`

	scaffoldPath := filepath.Join(atmosDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldPath, []byte(scaffoldConfig), 0644); err != nil {
		t.Fatalf("Failed to create scaffold.yaml: %v", err)
	}

	// Test validation
	err := validateScaffoldTemplate(tempDir, "test-template")
	if err == nil {
		t.Error("Expected validation to fail due to template mismatch")
	}
}

func TestValidateScaffoldTemplate_NoScaffoldFile(t *testing.T) {
	tempDir := t.TempDir()

	// Test validation without scaffold.yaml (should pass)
	err := validateScaffoldTemplate(tempDir, "test-template")
	if err != nil {
		t.Errorf("Expected validation to pass when no scaffold.yaml exists, got error: %v", err)
	}
}

func TestValidateScaffoldTemplate_MissingTemplateKey(t *testing.T) {
	tempDir := t.TempDir()

	// Create .atmos directory
	atmosDir := filepath.Join(tempDir, ".atmos")
	if err := os.MkdirAll(atmosDir, 0755); err != nil {
		t.Fatalf("Failed to create .atmos directory: %v", err)
	}

	// Create scaffold.yaml without template key
	scaffoldConfig := `values:
  name: "test-project"`

	scaffoldPath := filepath.Join(atmosDir, "scaffold.yaml")
	if err := os.WriteFile(scaffoldPath, []byte(scaffoldConfig), 0644); err != nil {
		t.Fatalf("Failed to create scaffold.yaml: %v", err)
	}

	// Test validation
	err := validateScaffoldTemplate(tempDir, "test-template")
	if err == nil {
		t.Error("Expected validation to fail due to missing template key")
	}
}

func TestGenerateFromLocal_ValidTemplate(t *testing.T) {
	// Create a temporary template directory
	tempDir := t.TempDir()
	templateDir := filepath.Join(tempDir, "local-template")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template directory: %v", err)
	}

	// Create a simple template file
	templateFile := filepath.Join(templateDir, "README.md")
	if err := os.WriteFile(templateFile, []byte("# Local Template"), 0644); err != nil {
		t.Fatalf("Failed to create template file: %v", err)
	}

	// Test generateFromLocal
	ui := ui.NewInitUI()
	targetPath := filepath.Join(tempDir, "output")
	err := generateFromLocal(templateDir, targetPath, false, false, true, map[string]interface{}{
		"name": "test-project",
	}, ui, []string{"{{", "}}"})
	if err != nil {
		t.Fatalf("Failed to generate from local template: %v", err)
	}

	// Verify output was created
	outputFile := filepath.Join(targetPath, "README.md")
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected README.md to be created")
	}
}

func TestGenerateFromLocal_NonexistentTemplate(t *testing.T) {
	// Test generateFromLocal with non-existent template
	ui := ui.NewInitUI()
	targetPath := filepath.Join(t.TempDir(), "output")
	err := generateFromLocal("/nonexistent/template", targetPath, false, false, true, make(map[string]interface{}), ui, []string{"{{", "}}"})
	if err == nil {
		t.Error("Expected error for non-existent template")
	}
}
