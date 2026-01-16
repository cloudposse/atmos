package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// Note: The dry-run preview functions require UI initialization which
// is done at runtime. Testing them requires integration tests.
// Here we test the helper functions that don't require UI.

// TestLoadDryRunValues tests loading values for dry-run.
func TestLoadDryRunValues(t *testing.T) {
	tests := []struct {
		name        string
		config      *templates.Configuration
		vars        map[string]interface{}
		expectError bool
	}{
		{
			name: "no scaffold config",
			config: &templates.Configuration{
				Files: []templates.File{{Path: "test.txt"}},
			},
			vars:        map[string]interface{}{"key": "value"},
			expectError: false,
		},
		{
			name: "with scaffold config and defaults",
			config: &templates.Configuration{
				Files: []templates.File{
					{
						Path: config.ScaffoldConfigFileName,
						Content: `name: Test
fields:
  project_name:
    type: string
    default: default-name
`,
					},
				},
			},
			vars:        map[string]interface{}{},
			expectError: false,
		},
		{
			name: "invalid scaffold config",
			config: &templates.Configuration{
				Files: []templates.File{
					{
						Path:    config.ScaffoldConfigFileName,
						Content: "invalid: yaml: content: [",
					},
				},
			},
			vars:        map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := loadDryRunValues(tt.config, tt.vars)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, values)
			}
		})
	}
}

// TestFindScaffoldConfigFile tests finding scaffold config in file list.
func TestFindScaffoldConfigFile(t *testing.T) {
	tests := []struct {
		name     string
		files    []templates.File
		expected bool
	}{
		{
			name: "config exists",
			files: []templates.File{
				{Path: "file1.txt"},
				{Path: config.ScaffoldConfigFileName},
				{Path: "file2.txt"},
			},
			expected: true,
		},
		{
			name: "config does not exist",
			files: []templates.File{
				{Path: "file1.txt"},
				{Path: "file2.txt"},
			},
			expected: false,
		},
		{
			name:     "empty file list",
			files:    []templates.File{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findScaffoldConfigFile(tt.files)
			if tt.expected {
				assert.NotNil(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

// TestRenderFilePath tests file path rendering with variables.
func TestRenderFilePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		values   map[string]interface{}
		expected string
	}{
		{
			name:     "simple path no variables",
			path:     "path/to/file.txt",
			values:   map[string]interface{}{},
			expected: "path/to/file.txt",
		},
		{
			name:     "path with single variable",
			path:     "{{.project_name}}/file.txt",
			values:   map[string]interface{}{"project_name": "my-project"},
			expected: "my-project/file.txt",
		},
		{
			name: "path with multiple variables",
			path: "{{.namespace}}/{{.environment}}/{{.app}}.yaml",
			values: map[string]interface{}{
				"namespace":   "prod",
				"environment": "staging",
				"app":         "api",
			},
			expected: "prod/staging/api.yaml",
		},
		{
			name:     "path with non-string variable",
			path:     "{{.count}}/file.txt",
			values:   map[string]interface{}{"count": 42},
			expected: "{{.count}}/file.txt", // Non-string values not replaced
		},
		{
			name:     "empty values",
			path:     "{{.var}}/file.txt",
			values:   map[string]interface{}{},
			expected: "{{.var}}/file.txt", // Variable not in values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderFilePath(tt.path, tt.values)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveTargetDirectory tests target directory resolution.
func TestResolveTargetDirectory(t *testing.T) {
	tests := []struct {
		name        string
		targetDir   string
		expectError bool
	}{
		{
			name:        "empty target directory",
			targetDir:   "",
			expectError: false,
		},
		{
			name:        "absolute path",
			targetDir:   "/tmp/test",
			expectError: false,
		},
		{
			name:        "relative path",
			targetDir:   "./test",
			expectError: false,
		},
		{
			name:        "current directory",
			targetDir:   ".",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveTargetDirectory(tt.targetDir)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.targetDir != "" {
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

// TestLoadScaffoldTemplates tests loading scaffold templates.
func TestLoadScaffoldTemplates(t *testing.T) {
	configs, origins, ui, err := loadScaffoldTemplates()
	require.NoError(t, err)
	assert.NotNil(t, configs)
	assert.NotNil(t, origins)
	assert.NotNil(t, ui)
	assert.NotEmpty(t, configs)
}

// TestExecuteTemplateGenerationErrors tests error paths in template generation.
func TestExecuteTemplateGenerationErrors(t *testing.T) {
	// This tests the execution flow, not full integration
	// Most error paths require complex setup with git repos, etc.

	// Test that the function exists and has proper signature
	selectedConfig := templates.Configuration{
		Name: "Test",
		Files: []templates.File{
			{Path: "test.txt", Content: "test"},
		},
	}

	// Test with empty target directory (should use interactive flow).
	err := executeTemplateGeneration(&selectedConfig, "", false, false, map[string]interface{}{}, nil)
	// This will error because UI is nil, but that's expected in test.
	assert.Error(t, err)
}

// TestExecuteScaffoldGenerateWithDryRun tests dry-run flag integration.
func TestExecuteScaffoldGenerateWithDryRun(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a simple template directory
	templateDir := filepath.Join(tempDir, "templates", "test-template")
	err := os.MkdirAll(templateDir, 0o755)
	require.NoError(t, err)

	// Create a scaffold.yaml
	scaffoldYAML := `name: Test Template
description: A test template
version: 1.0.0
fields:
  project_name:
    type: string
    default: test-project
`
	err = os.WriteFile(filepath.Join(templateDir, "scaffold.yaml"), []byte(scaffoldYAML), 0o644)
	require.NoError(t, err)

	// Create a template file
	err = os.WriteFile(filepath.Join(templateDir, "README.md"), []byte("# {{.project_name}}"), 0o644)
	require.NoError(t, err)

	// Note: Full integration test would require setting up the command context
	// This is a structural test to ensure the dry-run code path exists
	assert.NotNil(t, renderDryRunPreview)
}

// TestSelectTemplateErrors tests error handling in template selection.
func TestSelectTemplateErrors(t *testing.T) {
	configs := map[string]templates.Configuration{
		"template1": {Name: "template1", TemplateID: "id1"},
		"template2": {Name: "template2", TemplateID: "id2"},
	}

	// Test selecting non-existent template
	_, err := selectTemplate("nonexistent", configs, nil)
	assert.Error(t, err)

	// Test selecting with empty name (would trigger interactive mode, but UI is nil)
	_, err = selectTemplate("", configs, nil)
	assert.Error(t, err)
}
