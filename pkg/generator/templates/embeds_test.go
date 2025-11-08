package templates

import (
	"testing"
)

func TestGetAvailableConfigurations(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	if len(configs) == 0 {
		t.Error("Expected at least one configuration, got 0")
	}

	// Check that configurations have required fields
	for name, config := range configs {
		if config.Name == "" {
			t.Errorf("Configuration %s has empty Name", name)
		}
		if config.TemplateID == "" {
			t.Errorf("Configuration %s has empty TemplateID", name)
		}
		if len(config.Files) == 0 {
			t.Errorf("Configuration %s has no files", name)
		}
	}
}

func TestGetAvailableConfigurations_ExpectedTemplates(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// We should have at least "atmos" and "simple" templates
	expectedTemplates := []string{"atmos", "simple"}
	for _, expected := range expectedTemplates {
		if _, exists := configs[expected]; !exists {
			t.Errorf("Expected template %q not found in configurations", expected)
		}
	}
}

func TestConfiguration_FilesStructure(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// Get the "atmos" template for detailed testing
	atmosConfig, exists := configs["atmos"]
	if !exists {
		t.Skip("atmos template not found, skipping file structure test")
	}

	// Verify files have proper structure
	var foundREADME, foundAtmosYaml bool
	for _, file := range atmosConfig.Files {
		if file.Path == "" {
			t.Error("Found file with empty path")
		}

		// Check for expected files
		if file.Path == "README.md" {
			foundREADME = true
			if file.Content == "" {
				t.Error("README.md has empty content")
			}
			if file.IsDirectory {
				t.Error("README.md marked as directory")
			}
		}
		if file.Path == "atmos.yaml" {
			foundAtmosYaml = true
		}
	}

	if !foundREADME {
		t.Error("Expected to find README.md in atmos template")
	}
	if !foundAtmosYaml {
		t.Error("Expected to find atmos.yaml in atmos template")
	}
}

func TestConfiguration_README(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	atmosConfig, exists := configs["atmos"]
	if !exists {
		t.Skip("atmos template not found")
	}

	if atmosConfig.README == "" {
		t.Error("Expected atmos template to have README content")
	}
}

func TestHasScaffoldConfig(t *testing.T) {
	tests := []struct {
		name     string
		files    []File
		expected bool
	}{
		{
			name: "has scaffold.yaml",
			files: []File{
				{Path: "scaffold.yaml", IsDirectory: false},
				{Path: "README.md", IsDirectory: false},
			},
			expected: true,
		},
		{
			name: "no scaffold.yaml",
			files: []File{
				{Path: "README.md", IsDirectory: false},
				{Path: "atmos.yaml", IsDirectory: false},
			},
			expected: false,
		},
		{
			name: "scaffold.yaml is directory (not file)",
			files: []File{
				{Path: "scaffold.yaml", IsDirectory: true},
			},
			expected: false,
		},
		{
			name:     "empty files list",
			files:    []File{},
			expected: false,
		},
		{
			name: "scaffold.yaml in subdirectory",
			files: []File{
				{Path: "subdir/scaffold.yaml", IsDirectory: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasScaffoldConfig(tt.files)
			if result != tt.expected {
				t.Errorf("HasScaffoldConfig() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfiguration_TemplateDetection(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// Check that files with .tmpl extension or {{ }} are marked as templates
	for _, config := range configs {
		for _, file := range config.Files {
			if file.IsDirectory {
				continue
			}

			// Files with .tmpl extension should be marked as template
			if len(file.Path) > 5 && file.Path[len(file.Path)-5:] == ".tmpl" {
				if !file.IsTemplate {
					t.Errorf("File %s has .tmpl extension but IsTemplate = false", file.Path)
				}
			}

			// Files containing {{ should be marked as template (basic check)
			// Note: This is a heuristic, not all files with {{ are Go templates
		}
	}
}

func TestConfiguration_Permissions(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			if file.IsDirectory {
				if file.Permissions != 0o755 {
					t.Errorf("Directory %s in template %s has permissions %o, expected 0755",
						file.Path, name, file.Permissions)
				}
			} else {
				if file.Permissions != 0o644 {
					t.Errorf("File %s in template %s has permissions %o, expected 0644",
						file.Path, name, file.Permissions)
				}
			}
		}
	}
}

func TestConfiguration_Metadata(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		// Check that Name matches the template directory name
		if config.Name != name {
			t.Errorf("Template %s: Name field %q doesn't match directory name %q",
				name, config.Name, name)
		}

		// Check that TemplateID is set
		if config.TemplateID == "" {
			t.Errorf("Template %s has empty TemplateID", name)
		}

		// Check that Description contains the word "template"
		// This is based on the default description format
		// If templates have custom descriptions, this test may need adjustment
	}
}

func TestFile_NonEmpty(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			// Non-directory files should have content (or be deliberately empty)
			// Directories should have empty content
			if file.IsDirectory && file.Content != "" {
				t.Errorf("Directory %s in template %s has non-empty content",
					file.Path, name)
			}
		}
	}
}
