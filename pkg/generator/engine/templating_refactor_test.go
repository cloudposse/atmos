package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/project/config"
)

// TestExtractDelimiters tests the extractDelimiters function.
func TestExtractDelimiters(t *testing.T) {
	tests := []struct {
		name           string
		scaffoldConfig interface{}
		expected       []string
	}{
		{
			name:           "nil config returns defaults",
			scaffoldConfig: nil,
			expected:       []string{"{{", "}}"},
		},
		{
			name: "pointer to ScaffoldConfig with delimiters",
			scaffoldConfig: &config.ScaffoldConfig{
				Delimiters: []string{"[[", "]]"},
			},
			expected: []string{"[[", "]]"},
		},
		{
			name: "ScaffoldConfig value with delimiters",
			scaffoldConfig: config.ScaffoldConfig{
				Delimiters: []string{"<%", "%>"},
			},
			expected: []string{"<%", "%>"},
		},
		{
			name: "map with delimiters",
			scaffoldConfig: map[string]interface{}{
				"delimiters": []interface{}{"<<", ">>"},
			},
			expected: []string{"<<", ">>"},
		},
		{
			name: "ScaffoldConfig with wrong number of delimiters",
			scaffoldConfig: &config.ScaffoldConfig{
				Delimiters: []string{"[["},
			},
			expected: []string{"{{", "}}"},
		},
		{
			name: "ScaffoldConfig with empty delimiters",
			scaffoldConfig: &config.ScaffoldConfig{
				Delimiters: []string{},
			},
			expected: []string{"{{", "}}"},
		},
		{
			name:           "empty map returns defaults",
			scaffoldConfig: map[string]interface{}{},
			expected:       []string{"{{", "}}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDelimiters(tt.scaffoldConfig)

			if len(result) != 2 {
				t.Errorf("Expected 2 delimiters, got %d", len(result))
			}

			if result[0] != tt.expected[0] || result[1] != tt.expected[1] {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestProcessFilePath tests the processFilePath method.
func TestProcessFilePath(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		filePath    string
		userValues  map[string]interface{}
		expected    string
		expectError bool
	}{
		{
			name:        "no user values returns original path",
			filePath:    "config/app.yaml",
			userValues:  nil,
			expected:    "config/app.yaml",
			expectError: false,
		},
		{
			name:     "simple template in path",
			filePath: "{{.Config.env}}/app.yaml",
			userValues: map[string]interface{}{
				"env": "production",
			},
			expected:    "production/app.yaml",
			expectError: false,
		},
		{
			name:     "multiple templates in path",
			filePath: "{{.Config.namespace}}/{{.Config.env}}/config.yaml",
			userValues: map[string]interface{}{
				"namespace": "my-app",
				"env":       "staging",
			},
			expected:    "my-app/staging/config.yaml",
			expectError: false,
		},
		{
			name:     "invalid template syntax",
			filePath: "{{.Config.env/app.yaml",
			userValues: map[string]interface{}{
				"env": "prod",
			},
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.processFilePath(tt.filePath, "/tmp", nil, tt.userValues, []string{"{{", "}}"})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %s, got %s", tt.expected, result)
				}
			}
		})
	}
}

// TestEnsureDirectory tests the ensureDirectory function.
func TestEnsureDirectory(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		fullPath    string
		expectError bool
	}{
		{
			name:        "create single level directory",
			fullPath:    filepath.Join(tempDir, "test", "file.txt"),
			expectError: false,
		},
		{
			name:        "create nested directories",
			fullPath:    filepath.Join(tempDir, "a", "b", "c", "file.txt"),
			expectError: false,
		},
		{
			name:        "existing directory",
			fullPath:    filepath.Join(tempDir, "file.txt"),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureDirectory(tt.fullPath)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify directory was created
				dir := filepath.Dir(tt.fullPath)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					t.Errorf("Directory %s was not created", dir)
				}
			}
		})
	}
}

// TestFileExists tests the fileExists function.
func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	existingFile := filepath.Join(tempDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "existing file",
			path:     existingFile,
			expected: true,
		},
		{
			name:     "non-existing file",
			path:     filepath.Join(tempDir, "does-not-exist.txt"),
			expected: false,
		},
		{
			name:     "directory exists",
			path:     tempDir,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fileExists(tt.path)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestHandleExistingFile tests the handleExistingFile method.
func TestHandleExistingFile(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// Create an existing file
	existingPath := filepath.Join(tempDir, "existing.txt")
	if err := os.WriteFile(existingPath, []byte("existing content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		force       bool
		update      bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "no force or update flag",
			force:       false,
			update:      false,
			expectError: true,
			errorMsg:    "file already exists",
		},
		{
			name:        "force flag allows overwrite",
			force:       true,
			update:      false,
			expectError: false,
		},
		{
			name:        "update flag triggers merge",
			force:       false,
			update:      true,
			expectError: false,
		},
		{
			name:        "both flags - update takes precedence",
			force:       true,
			update:      true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := File{
				Path:        "test.txt",
				Content:     "new content",
				IsTemplate:  false,
				Permissions: 0o644,
			}

			err := processor.handleExistingFile(file, existingPath, tempDir, tt.force, tt.update, nil, nil, []string{"{{", "}}"})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestProcessFileContent tests the processFileContent method.
func TestProcessFileContent(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		file        File
		userValues  map[string]interface{}
		expected    string
		expectError bool
	}{
		{
			name: "no template processing needed",
			file: File{
				Path:       "test.txt",
				Content:    "static content",
				IsTemplate: false,
			},
			userValues:  nil,
			expected:    "static content",
			expectError: false,
		},
		{
			name: "process template with user values",
			file: File{
				Path:       "test.txt",
				Content:    "Hello {{.Config.name}}!",
				IsTemplate: false,
			},
			userValues: map[string]interface{}{
				"name": "World",
			},
			expected:    "Hello World!",
			expectError: false,
		},
		{
			name: "process file marked as template",
			file: File{
				Path:       "test.txt",
				Content:    "Value: {{.Config.value}}",
				IsTemplate: true,
			},
			userValues: map[string]interface{}{
				"value": "42",
			},
			expected:    "Value: 42",
			expectError: false,
		},
		{
			name: "invalid template syntax",
			file: File{
				Path:       "test.txt",
				Content:    "Invalid: {{.Config.value",
				IsTemplate: true,
			},
			userValues: map[string]interface{}{
				"value": "test",
			},
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.processFileContent(tt.file, "/tmp", nil, tt.userValues, []string{"{{", "}}"})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

// TestWriteNewFile tests the writeNewFile method.
func TestWriteNewFile(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		file        File
		userValues  map[string]interface{}
		expectError bool
	}{
		{
			name: "write simple file",
			file: File{
				Path:        "simple.txt",
				Content:     "simple content",
				IsTemplate:  false,
				Permissions: 0o644,
			},
			userValues:  nil,
			expectError: false,
		},
		{
			name: "write template file",
			file: File{
				Path:        "template.txt",
				Content:     "Name: {{.Config.name}}",
				IsTemplate:  false,
				Permissions: 0o600,
			},
			userValues: map[string]interface{}{
				"name": "Test",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fullPath := filepath.Join(tempDir, tt.file.Path)

			err := processor.writeNewFile(tt.file, fullPath, tempDir, nil, tt.userValues, []string{"{{", "}}"})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// Verify file was created
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					t.Error("File was not created")
				}

				// Verify permissions (skip on Windows where permissions work differently)
				if runtime.GOOS != "windows" {
					info, err := os.Stat(fullPath)
					if err != nil {
						t.Fatalf("Failed to stat file: %v", err)
					}
					if info.Mode().Perm() != tt.file.Permissions {
						t.Errorf("Expected permissions %o, got %o", tt.file.Permissions, info.Mode().Perm())
					}
				}
			}
		})
	}
}
