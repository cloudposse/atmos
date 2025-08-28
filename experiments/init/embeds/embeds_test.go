package embeds

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/internal/types"
)

func TestGetAvailableConfigurations(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(configs) == 0 {
		t.Fatal("Expected at least one configuration")
	}

	// Check for expected configurations
	expectedConfigs := []string{
		"atmos.yaml",
		".editorconfig",
		".gitignore",
		"examples/demo-stacks",
		"examples/demo-localstack",
		"rich-project",
	}

	found := make(map[string]bool)
	for name := range configs {
		found[name] = true
	}

	for _, expected := range expectedConfigs {
		if !found[expected] {
			t.Errorf("Expected configuration %s not found", expected)
		}
	}
}

func TestGetConfiguration(t *testing.T) {
	// Test existing configuration
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["atmos.yaml"]
	if !exists {
		t.Fatal("Expected 'atmos.yaml' config to exist")
	}

	if config.Name != "atmos.yaml" {
		t.Errorf("Expected config name 'atmos.yaml', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected atmos.yaml config to have files")
	}
}

func TestGetConfiguration_NotFound(t *testing.T) {
	// Test non-existing configuration
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	_, exists := configs["nonexistent"]
	if exists {
		t.Fatal("Expected 'nonexistent' config to not exist")
	}
}

func TestGetConfiguration_AtmosYAML(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["atmos.yaml"]
	if !exists {
		t.Fatal("Expected 'atmos.yaml' config to exist")
	}

	if config.Name != "atmos.yaml" {
		t.Errorf("Expected config name 'atmos.yaml', got %s", config.Name)
	}

	if len(config.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(config.Files))
	}

	file := config.Files[0]
	if file.Path != "atmos.yaml" {
		t.Errorf("Expected file path 'atmos.yaml', got %s", file.Path)
	}

	if !file.IsTemplate {
		t.Error("Expected atmos.yaml to be a template")
	}
}

func TestGetConfiguration_EditorConfig(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs[".editorconfig"]
	if !exists {
		t.Fatal("Expected '.editorconfig' config to exist")
	}

	if config.Name != ".editorconfig" {
		t.Errorf("Expected config name '.editorconfig', got %s", config.Name)
	}

	if len(config.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(config.Files))
	}

	// Find the .editorconfig file
	var editorConfigFile *types.File
	for i := range config.Files {
		if config.Files[i].Path == ".editorconfig" {
			editorConfigFile = &config.Files[i]
			break
		}
	}

	if editorConfigFile == nil {
		t.Error("Expected .editorconfig file not found")
	} else {
		if editorConfigFile.IsTemplate {
			t.Error("Expected .editorconfig to not be a template (static file)")
		}
	}
}

func TestGetConfiguration_Gitignore(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs[".gitignore"]
	if !exists {
		t.Fatal("Expected '.gitignore' config to exist")
	}

	if config.Name != ".gitignore" {
		t.Errorf("Expected config name '.gitignore', got %s", config.Name)
	}

	if len(config.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(config.Files))
	}

	// Find the .gitignore file
	var gitignoreFile *types.File
	for i := range config.Files {
		if config.Files[i].Path == ".gitignore" {
			gitignoreFile = &config.Files[i]
			break
		}
	}

	if gitignoreFile == nil {
		t.Error("Expected .gitignore file not found")
	} else {
		if gitignoreFile.IsTemplate {
			t.Error("Expected .gitignore to not be a template (static file)")
		}
	}
}

func TestGetConfiguration_RichProject(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["rich-project"]
	if !exists {
		t.Fatal("Expected 'rich-project' config to exist")
	}

	if config.Name != "Atmos Scaffold Template Configuration" {
		t.Errorf("Expected config name 'Atmos Scaffold Template Configuration', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected rich-project config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"README.md", "scaffold.yaml"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in rich-project", expected)
		}
	}
}

func TestGetConfiguration_SimpleScaffold(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["simple-scaffold"]
	if !exists {
		t.Fatal("Expected 'simple-scaffold' config to exist")
	}

	if config.Name != "Simple Scaffold Configuration" {
		t.Errorf("Expected config name 'Simple Scaffold Configuration', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected simple-scaffold config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"README.md", "scaffold.yaml"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in simple-scaffold", expected)
		}
	}
}

func TestGetConfiguration_PathTest(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["path-test"]
	if !exists {
		t.Fatal("Expected 'path-test' config to exist")
	}

	if config.Name != "Path Template Test" {
		t.Errorf("Expected config name 'Path Template Test', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected path-test config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"scaffold.yaml", "{{.Config.namespace}}-monitoring.yaml", "{{.Config.namespace}}/config.yaml", "{{.Config.namespace}}/{{.Config.subdirectory}}/deep.yaml", "{{.Config.namespace}}/docs/README.md"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in path-test", expected)
		}
	}
}

func TestFilePermissions(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			// Check that permissions are reasonable
			if file.Permissions == 0 {
				t.Errorf("File %s in config %s has no permissions set", file.Path, name)
			}

			// Check that permissions are in valid range
			if file.Permissions < 0400 || file.Permissions > 0777 {
				t.Errorf("File %s in config %s has invalid permissions: %o", file.Path, name, file.Permissions)
			}
		}
	}
}

func TestFileContent(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			// Check that content is not empty
			if file.Content == "" {
				t.Errorf("File %s in config %s has empty content", file.Path, name)
			}

			// Check that content is not just whitespace
			if len(strings.TrimSpace(file.Content)) == 0 {
				t.Errorf("File %s in config %s has only whitespace content", file.Path, name)
			}
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
