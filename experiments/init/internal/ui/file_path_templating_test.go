package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
)

func TestProcessFilePath(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userValues := map[string]interface{}{
		"namespace": "production",
		"author":    "test-user",
		"region":    "us-east-1",
	}

	testCases := []struct {
		name         string
		inputPath    string
		expectedPath string
		shouldError  bool
	}{
		{
			name:         "simple namespace substitution",
			inputPath:    "{{.Config.namespace}}/config.yaml",
			expectedPath: "production/config.yaml",
			shouldError:  false,
		},
		{
			name:         "nested path with multiple variables",
			inputPath:    "{{.Config.namespace}}/{{.Config.region}}/app.yaml",
			expectedPath: "production/us-east-1/app.yaml",
			shouldError:  false,
		},
		{
			name:         "no template syntax",
			inputPath:    "static/config.yaml",
			expectedPath: "static/config.yaml",
			shouldError:  false,
		},
		{
			name:         "project name in path",
			inputPath:    "{{.ProjectName}}/{{.Config.namespace}}/config.yaml",
			expectedPath: filepath.Join(filepath.Base(tempDir), "production", "config.yaml"),
			shouldError:  false,
		},
		{
			name:         "invalid template syntax",
			inputPath:    "{{.Config.nonexistent}}/config.yaml",
			expectedPath: "",
			shouldError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ui.processFilePath(tc.inputPath, tempDir, nil, userValues)

			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expectedPath {
					t.Errorf("Expected path %s, got %s", tc.expectedPath, result)
				}
			}
		})
	}
}

func TestProcessFilePathWithConfig(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userConfig := &config.Config{
		Author:  "test-author",
		Year:    "2024",
		License: "MIT",
	}

	testCases := []struct {
		name         string
		inputPath    string
		expectedPath string
		shouldError  bool
	}{
		{
			name:         "author in path",
			inputPath:    "{{.Config.author}}/config.yaml",
			expectedPath: "test-author/config.yaml",
			shouldError:  false,
		},
		{
			name:         "year in path",
			inputPath:    "archives/{{.Config.year}}/backup.yaml",
			expectedPath: "archives/2024/backup.yaml",
			shouldError:  false,
		},
		{
			name:         "license in path",
			inputPath:    "licenses/{{.Config.license}}/LICENSE",
			expectedPath: "licenses/MIT/LICENSE",
			shouldError:  false,
		},
		{
			name:         "mixed variables",
			inputPath:    "{{.Config.author}}/{{.Config.year}}/project.yaml",
			expectedPath: "test-author/2024/project.yaml",
			shouldError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ui.processFilePathWithConfig(tc.inputPath, tempDir, userConfig)

			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expectedPath {
					t.Errorf("Expected path %s, got %s", tc.expectedPath, result)
				}
			}
		})
	}
}

func TestProcessFileWithTemplating_FilePathTemplating(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userValues := map[string]interface{}{
		"namespace": "testing",
		"author":    "test-user",
	}

	file := embeds.File{
		Path:        "{{.Config.namespace}}/config.yaml",
		Content:     "namespace: {{.Config.namespace}}\nauthor: {{.Config.author}}",
		IsTemplate:  true,
		Permissions: 0644,
	}

	err := ui.processFileWithTemplating(file, tempDir, false, false, nil, userValues)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that the file was created with the correct path
	expectedPath := filepath.Join(tempDir, "testing", "config.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s was not created", expectedPath)
	}

	// Check that the content was processed correctly
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	expectedContent := "namespace: testing\nauthor: test-user"
	if string(content) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(content))
	}
}

func TestProcessFileWithBasicTemplating_FilePathTemplating(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userConfig := &config.Config{
		Author:  "basic-author",
		Year:    "2024",
		License: "Apache-2.0",
	}

	file := embeds.File{
		Path:        "{{.Config.author}}/{{.Config.year}}/config.yaml",
		Content:     "author: {{.Author}}\nyear: {{.Year}}",
		IsTemplate:  true,
		Permissions: 0644,
	}

	err := ui.processFileWithBasicTemplating(file, tempDir, false, false, userConfig)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that the file was created with the correct path
	expectedPath := filepath.Join(tempDir, "basic-author", "2024", "config.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s was not created", expectedPath)
	}

	// Check that the content was processed correctly
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	expectedContent := "author: basic-author\nyear: 2024"
	if string(content) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(content))
	}
}

func TestFilePathTemplating_ErrorCases(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	testCases := []struct {
		name        string
		filePath    string
		userValues  map[string]interface{}
		expectError bool
	}{
		{
			name:     "missing variable evaluates to empty",
			filePath: "{{.Config.missing}}/config.yaml",
			userValues: map[string]interface{}{
				"namespace": "test",
			},
			expectError: false, // Missing variables are handled gracefully
		},
		{
			name:        "invalid template syntax",
			filePath:    "{{.Config.namespace/config.yaml",
			userValues:  map[string]interface{}{"namespace": "test"},
			expectError: true,
		},
		{
			name:        "empty path result",
			filePath:    "{{.Config.empty}}/config.yaml",
			userValues:  map[string]interface{}{"empty": ""},
			expectError: false, // Empty string is valid
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ui.processFilePath(tc.filePath, tempDir, nil, tc.userValues)

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestFilePathTemplating_NoTemplatesProcessed(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userValues := map[string]interface{}{
		"namespace": "test",
	}

	// Test that paths without template syntax are not processed
	file := embeds.File{
		Path:        "static/config.yaml",
		Content:     "static content",
		IsTemplate:  false,
		Permissions: 0644,
	}

	err := ui.processFileWithTemplating(file, tempDir, false, false, nil, userValues)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that the file was created with the original path
	expectedPath := filepath.Join(tempDir, "static", "config.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file at %s was not created", expectedPath)
	}
}

func TestFilePathTemplating_SpecialCharacters(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	userValues := map[string]interface{}{
		"namespace": "my-namespace",
		"env":       "dev_test",
	}

	testCases := []struct {
		name         string
		inputPath    string
		expectedPath string
	}{
		{
			name:         "dash in namespace",
			inputPath:    "{{.Config.namespace}}/config.yaml",
			expectedPath: "my-namespace/config.yaml",
		},
		{
			name:         "underscore in env",
			inputPath:    "{{.Config.env}}/{{.Config.namespace}}/app.yaml",
			expectedPath: "dev_test/my-namespace/app.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ui.processFilePath(tc.inputPath, tempDir, nil, userValues)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tc.expectedPath {
				t.Errorf("Expected path %s, got %s", tc.expectedPath, result)
			}
		})
	}
}

func TestFilePathTemplating_ConditionalCreation(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	testCases := []struct {
		name         string
		file         embeds.File
		userValues   map[string]interface{}
		shouldCreate bool
		expectedPath string
	}{
		{
			name: "conditional file created when condition is true",
			file: embeds.File{
				Path:        "{{if .Config.enable_feature}}feature/config.yaml{{end}}",
				Content:     "feature enabled",
				IsTemplate:  false,
				Permissions: 0644,
			},
			userValues: map[string]interface{}{
				"enable_feature": true,
			},
			shouldCreate: true,
			expectedPath: "feature/config.yaml",
		},
		{
			name: "conditional file not created when condition is false",
			file: embeds.File{
				Path:        "{{if .Config.enable_feature}}feature/config.yaml{{end}}",
				Content:     "feature enabled",
				IsTemplate:  false,
				Permissions: 0644,
			},
			userValues: map[string]interface{}{
				"enable_feature": false,
			},
			shouldCreate: false,
			expectedPath: "",
		},
		{
			name: "conditional file not created when variable is missing",
			file: embeds.File{
				Path:        "{{if .Config.missing_var}}feature/config.yaml{{end}}",
				Content:     "feature enabled",
				IsTemplate:  false,
				Permissions: 0644,
			},
			userValues: map[string]interface{}{
				"other_var": "value",
			},
			shouldCreate: false,
			expectedPath: "",
		},
		{
			name: "cloud provider specific file created for aws",
			file: embeds.File{
				Path:        "{{if eq .Config.cloud_provider \"aws\"}}aws/main.tf{{end}}",
				Content:     "aws config",
				IsTemplate:  false,
				Permissions: 0644,
			},
			userValues: map[string]interface{}{
				"cloud_provider": "aws",
			},
			shouldCreate: true,
			expectedPath: "aws/main.tf",
		},
		{
			name: "cloud provider specific file not created for gcp when aws is selected",
			file: embeds.File{
				Path:        "{{if eq .Config.cloud_provider \"gcp\"}}gcp/main.tf{{end}}",
				Content:     "gcp config",
				IsTemplate:  false,
				Permissions: 0644,
			},
			userValues: map[string]interface{}{
				"cloud_provider": "aws",
			},
			shouldCreate: false,
			expectedPath: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up temp directory for each test
			testDir := filepath.Join(tempDir, tc.name)
			os.RemoveAll(testDir)
			os.MkdirAll(testDir, 0755)

						err := ui.processFileWithTemplating(tc.file, testDir, false, false, nil, tc.userValues)
			
			if tc.shouldCreate {
				if err != nil {
					t.Fatalf("Unexpected error when file should be created: %v", err)
				}
				// Check that the file was created
				expectedPath := filepath.Join(testDir, tc.expectedPath)
				if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
					t.Errorf("Expected file at %s was not created", tc.expectedPath)
				}
			} else {
				// File should be skipped, expect FileSkippedError
				if err == nil {
					t.Errorf("Expected FileSkippedError but got no error")
				} else if !IsFileSkipped(err) {
					t.Errorf("Expected FileSkippedError but got: %v", err)
				}
				
				// Check that no files were created (directory should be empty or contain only directories)
				entries, err := os.ReadDir(testDir)
				if err != nil {
					t.Fatalf("Failed to read test directory: %v", err)
				}
				
				fileCount := 0
				for _, entry := range entries {
					if !entry.IsDir() {
						fileCount++
					}
				}
				
				if fileCount > 0 {
					t.Errorf("Expected no files to be created, but found %d files", fileCount)
				}
			}
		})
	}
}
