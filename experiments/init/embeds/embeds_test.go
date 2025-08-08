package embeds

import (
	"strings"
	"testing"
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
	expectedConfigs := []string{"default", "atmos.yaml", ".editorconfig", ".gitignore"}
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

	config, exists := configs["default"]
	if !exists {
		t.Fatal("Expected 'default' config to exist")
	}

	if config.Name != "default" {
		t.Errorf("Expected config name 'default', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected default config to have files")
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

	if len(config.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(config.Files))
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

	if len(config.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(config.Files))
	}

	file := config.Files[0]
	if file.Path != ".editorconfig" {
		t.Errorf("Expected file path '.editorconfig', got %s", file.Path)
	}

	if file.IsTemplate {
		t.Error("Expected .editorconfig to not be a template")
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

	if len(config.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(config.Files))
	}

	file := config.Files[0]
	if file.Path != ".gitignore" {
		t.Errorf("Expected file path '.gitignore', got %s", file.Path)
	}

	if file.IsTemplate {
		t.Error("Expected .gitignore to not be a template")
	}
}

func TestGetConfiguration_DemoStacks(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["examples/demo-stacks"]
	if !exists {
		t.Fatal("Expected 'examples/demo-stacks' config to exist")
	}

	if config.Name != "examples/demo-stacks" {
		t.Errorf("Expected config name 'examples/demo-stacks', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected demo-stacks config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"stack.yaml", "vpc.tf", "README.md"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in demo-stacks", expected)
		}
	}
}

func TestGetConfiguration_DemoLocalstack(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["examples/demo-localstack"]
	if !exists {
		t.Fatal("Expected 'examples/demo-localstack' config to exist")
	}

	if config.Name != "examples/demo-localstack" {
		t.Errorf("Expected config name 'examples/demo-localstack', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected demo-localstack config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"docker-compose.yml", "atmos.yaml", "README.md"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in demo-localstack", expected)
		}
	}
}

func TestGetConfiguration_DemoHelmfile(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	config, exists := configs["examples/demo-helmfile"]
	if !exists {
		t.Fatal("Expected 'examples/demo-helmfile' config to exist")
	}

	if config.Name != "examples/demo-helmfile" {
		t.Errorf("Expected config name 'examples/demo-helmfile', got %s", config.Name)
	}

	if len(config.Files) == 0 {
		t.Error("Expected demo-helmfile config to have files")
	}

	// Check for expected files
	expectedFiles := []string{"component.yaml", "stack.yaml", "README.md"}
	found := make(map[string]bool)

	for _, file := range config.Files {
		found[file.Path] = true
	}

	for _, expected := range expectedFiles {
		if !found[expected] {
			t.Errorf("Expected file %s not found in demo-helmfile", expected)
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
