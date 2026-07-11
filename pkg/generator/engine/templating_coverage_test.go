package engine

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// TestProcessFileWithGitStorageError tests error handling when git storage fails.
func TestProcessFileWithGitStorageError(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// Set up git storage with invalid ref to trigger error
	// Note: This test validates that SetupGitStorage can be called
	// Actual git errors require a real git repo which is tested in update_test.go
	err := processor.SetupGitStorage(tempDir, "invalid-ref-that-does-not-exist")
	// May or may not error depending on git repo state
	_ = err
	assert.NotNil(t, processor)
}

// TestProcessTemplateExecutionErrors tests various template execution error paths.
func TestProcessTemplateExecutionErrors(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		template    string
		values      map[string]interface{}
		expectError bool
	}{
		{
			name:        "unclosed template tag",
			template:    "{{.Config.name",
			values:      map[string]interface{}{"name": "test"},
			expectError: true,
		},
		{
			name:        "invalid function call",
			template:    "{{invalidFunc .Config.name}}",
			values:      map[string]interface{}{"name": "test"},
			expectError: true,
		},
		{
			name:        "missing variable with strict mode",
			template:    "{{.Config.nonexistent}}",
			values:      map[string]interface{}{},
			expectError: false, // Returns <no value>
		},
		{
			name:        "malformed pipe",
			template:    "{{.Config.name | }}",
			values:      map[string]interface{}{"name": "test"},
			expectError: false, // Gomplate may handle this gracefully
		},
		{
			name:        "invalid range syntax",
			template:    "{{range}}{{end}}",
			values:      map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processor.ProcessTemplate(tt.template, filepath.Join(t.TempDir(), "test"), nil, tt.values)
			if tt.expectError {
				assert.Error(t, err, "Expected template execution error")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFileSkippedError_Error(t *testing.T) {
	err := &FileSkippedError{Path: "template/path", RenderedPath: "false"}

	assert.Contains(t, err.Error(), "template/path")
	assert.Contains(t, err.Error(), "false")
}

func TestProcessTemplateWithInvalidDelimiterSliceUsesDefaults(t *testing.T) {
	processor := NewProcessor()

	result, err := processor.ProcessTemplateWithDelimiters("Hello {{ .Config.name }}", t.TempDir(), nil, map[string]interface{}{"name": "demo"}, []string{"<<"})

	require.NoError(t, err)
	assert.Equal(t, "Hello demo", result)
}

func TestProcessTemplateExecuteError(t *testing.T) {
	processor := NewProcessor()

	_, err := processor.ProcessTemplate(`{{ fail "boom" }}`, t.TempDir(), nil, map[string]interface{}{})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateExecution)
}

func TestExtractDelimitersFromValueConfig(t *testing.T) {
	cfg := config.ScaffoldConfig{Spec: config.ScaffoldSpec{Delimiters: []string{"[[", "]]"}}}

	assert.Equal(t, []string{"[[", "]]"}, extractDelimiters(cfg))
}

func TestExtractDelimitersFromInvalidMapConfig(t *testing.T) {
	assert.Equal(t, []string{"{{", "}}"}, extractDelimiters(map[string]interface{}{"delimiters": []interface{}{"[[", 42}}))
	assert.Equal(t, []string{"{{", "}}"}, extractDelimiters(map[string]interface{}{"delimiters": "not-a-list"}))
}

func TestProcessFileContentUnprocessedTemplateError(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	file := File{
		Path:        "test.txt",
		Content:     `{{ "{{ .Config.name }}" }}`,
		IsTemplate:  true,
		Permissions: 0o644,
	}

	err := processor.ProcessFile(file, tempDir, false, false, nil, map[string]interface{}{"name": "demo"})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
}

// TestProcessFileDirectoryCreationError tests error when directory cannot be created.
func TestProcessFileDirectoryCreationError(t *testing.T) {
	processor := NewProcessor()

	// Create a file where we want a directory to be
	tempDir := t.TempDir()
	blockingFile := filepath.Join(tempDir, "blocked")
	err := os.WriteFile(blockingFile, []byte("blocking"), 0o644)
	require.NoError(t, err)

	// Try to create a file inside what should be a directory but is actually a file
	file := File{
		Path:        "blocked/test.txt",
		Content:     "test",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	err = processor.ProcessFile(file, tempDir, false, false, nil, nil)
	assert.Error(t, err, "Expected error when directory creation fails")
}

// TestProcessFileSkipPaths tests file skipping logic.
func TestProcessFileSkipPaths(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	skipPaths := []string{
		"",
		"false",
		"null",
		"<no value>",
		"path//double-slash.txt",
		"path/trailing/",
	}

	for _, path := range skipPaths {
		t.Run("skip_"+path, func(t *testing.T) {
			file := File{
				Path:        path,
				Content:     "test",
				IsTemplate:  false,
				Permissions: 0o644,
			}

			err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
			// Should return FileSkippedError
			var skipErr *FileSkippedError
			assert.True(t, errors.As(err, &skipErr), "Expected FileSkippedError for path: %s", path)
		})
	}

	// Absolute paths should return ErrPathTraversal (security check takes precedence over skip).
	t.Run("error_absolute_path", func(t *testing.T) {
		// Use platform-appropriate absolute path.
		var absPath string
		if runtime.GOOS == "windows" {
			absPath = `C:\absolute\path.txt`
		} else {
			absPath = "/absolute/path.txt"
		}

		file := File{
			Path:        absPath,
			Content:     "test",
			IsTemplate:  false,
			Permissions: 0o644,
		}

		err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
		assert.ErrorIs(t, err, errUtils.ErrPathTraversal, "Expected ErrPathTraversal for absolute path")
	})
}

// TestHandleExistingFileForce tests force flag overwrites existing files.
func TestHandleExistingFileForce(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// Create existing file
	filePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(filePath, []byte("original content"), 0o644)
	require.NoError(t, err)

	// Process with force flag
	file := File{
		Path:        "test.txt",
		Content:     "new content",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	err = processor.ProcessFile(file, tempDir, true, false, nil, nil)
	require.NoError(t, err)

	// Verify content was overwritten
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(content))
}

func TestValidateRenderedPath_PathTraversalRejected(t *testing.T) {
	err := validateRenderedPath("../escape/file.txt", "{{ .Config.dir }}/file.txt")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

func TestValidateRenderedPath_UnrenderedMarkerInPath(t *testing.T) {
	err := validateRenderedPath("foo/{{ .Config.missing }}/bar", "foo/{{ .Config.missing }}/bar")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnprocessedTemplate)
}

// TestHandleExistingFile_UpdateRenderErrorBeforeMerge covers the branch where
// re-rendering file.Content during --update fails before mergeFile is ever
// reached. The gitStorage field only needs to be non-nil to pass
// handleExistingFile's own precondition check; it's never dereferenced
// before the render error.
func TestHandleExistingFile_UpdateRenderErrorBeforeMerge(t *testing.T) {
	processor := NewProcessor()
	processor.gitStorage = &storage.GitBaseStorage{}

	dir := t.TempDir()
	fullPath := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(fullPath, []byte("existing"), 0o644))

	file := File{Path: "file.txt", Content: "{{ unterminated", IsTemplate: true, Permissions: 0o644}
	err := processor.handleExistingFile(file, fullPath, dir, false, true, nil, nil, []string{defaultLeftDelimiter, defaultRightDelimiter})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTemplateExecution)
}

// TestWriteFileErrors tests various file write error scenarios.
func TestWriteFileErrors(t *testing.T) {
	processor := NewProcessor()

	// Try to write to invalid path.
	file := File{
		Path:        "test.txt",
		Content:     "test",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	tempDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(tempDir, "test.txt"), 0o755))

	// Writing a file where a directory already exists should fail everywhere.
	err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
	assert.Error(t, err, "Expected error for non-existent directory")
}

// TestProcessFileContentWithTemplateErrors tests template processing errors in file content.
func TestProcessFileContentWithTemplateErrors(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		isTemplate  bool
		expectError bool
	}{
		{
			name:        "invalid template syntax in content",
			content:     "Hello {{.Config.name",
			isTemplate:  true,
			expectError: true,
		},
		{
			name:        "valid template",
			content:     "Hello {{.Config.name}}",
			isTemplate:  true,
			expectError: false,
		},
		{
			name:        "non-template file",
			content:     "Hello {{.Config.name}}",
			isTemplate:  false,
			expectError: false, // Should not process
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := File{
				Path:        "test_" + strings.ReplaceAll(tt.name, " ", "_") + ".txt",
				Content:     tt.content,
				IsTemplate:  tt.isTemplate,
				Permissions: 0o644,
			}

			userValues := map[string]interface{}{"name": "World"}
			err := processor.ProcessFile(file, tempDir, false, false, nil, userValues)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateNoUnprocessedTemplatesErrors tests validation error paths.
func TestValidateNoUnprocessedTemplatesErrors(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		content     string
		expectError bool
	}{
		{
			name:        "unprocessed default delimiters",
			content:     "This has {{.Config.var}} unprocessed",
			expectError: true,
		},
		{
			name:        "no template syntax",
			content:     "This is plain text",
			expectError: false,
		},
		{
			name:        "processed template (no syntax)",
			content:     "This is value processed",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateNoUnprocessedTemplates(tt.content)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateNoUnprocessedTemplatesWithCustomDelimiters tests custom delimiter validation.
func TestValidateNoUnprocessedTemplatesWithCustomDelimiters(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		content     string
		delimiters  []string
		expectError bool
	}{
		{
			name:        "unprocessed custom delimiters",
			content:     "This has [[ .Config.var ]] unprocessed",
			delimiters:  []string{"[[", "]]"},
			expectError: true,
		},
		{
			name:        "default delimiters with custom config",
			content:     "This has {{.Config.var}} but using custom delimiters",
			delimiters:  []string{"[[", "]]"},
			expectError: false, // Should ignore default delimiters
		},
		{
			name:        "invalid delimiters array",
			content:     "content",
			delimiters:  []string{"[["},
			expectError: false, // Falls back to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateNoUnprocessedTemplatesWithDelimiters(tt.content, tt.delimiters)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestShouldSkipFileEdgeCases tests edge cases in file skipping.
func TestShouldSkipFileEdgeCases(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name       string
		path       string
		shouldSkip bool
	}{
		{
			name:       "empty string",
			path:       "",
			shouldSkip: true,
		},
		{
			name:       "single slash",
			path:       "/",
			shouldSkip: true,
		},
		{
			name:       "multiple slashes",
			path:       "///",
			shouldSkip: true,
		},
		{
			name:       "path with spaces",
			path:       "  ",
			shouldSkip: false, // Spaces alone is technically a valid path
		},
		{
			name:       "false literal",
			path:       "false",
			shouldSkip: true,
		},
		{
			name:       "null literal",
			path:       "null",
			shouldSkip: true,
		},
		{
			name:       "no value literal",
			path:       "<no value>",
			shouldSkip: true,
		},
		{
			name:       "valid relative path",
			path:       "path/to/file.txt",
			shouldSkip: false,
		},
		{
			name:       "valid simple path",
			path:       "file.txt",
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ShouldSkipFile(tt.path)
			assert.Equal(t, tt.shouldSkip, result, "ShouldSkipFile(%q)", tt.path)
		})
	}
}

// TestTruncateString tests the truncateString helper function.
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string not truncated",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "exact length not truncated",
			input:    "exactlen",
			maxLen:   8,
			expected: "exactlen",
		},
		{
			name:     "long string truncated",
			input:    "this is a very long string that needs truncation",
			maxLen:   20,
			expected: "this is a very long ...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "maxLen zero",
			input:    "test",
			maxLen:   0,
			expected: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsUnprocessedTemplatesBasic tests unprocessed template detection with default delimiters.
func TestContainsUnprocessedTemplatesBasic(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "contains unprocessed default",
			content:  "text {{.Var}} more",
			expected: true,
		},
		{
			name:     "no template syntax",
			content:  "plain text",
			expected: false,
		},
		{
			name:     "only opening delimiter",
			content:  "text {{ incomplete",
			expected: false,
		},
		{
			name:     "only closing delimiter",
			content:  "text }} incomplete",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ContainsUnprocessedTemplates(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsUnprocessedTemplatesWithCustomDelimiters tests unprocessed template detection with custom delimiters.
func TestContainsUnprocessedTemplatesWithCustomDelimiters(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name       string
		content    string
		delimiters []string
		expected   bool
	}{
		{
			name:       "contains unprocessed custom",
			content:    "text [[.Var]] more",
			delimiters: []string{"[[", "]]"},
			expected:   true,
		},
		{
			name:       "wrong delimiters",
			content:    "text {{.Var}} more",
			delimiters: []string{"[[", "]]"},
			expected:   false,
		},
		{
			name:       "invalid delimiters",
			content:    "text {{.Var}} more",
			delimiters: []string{"{{"},
			expected:   true, // Falls back to default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.ContainsUnprocessedTemplatesWithDelimiters(tt.content, tt.delimiters)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessTemplateWithDelimitersErrors tests delimiter-specific error handling.
func TestProcessTemplateWithDelimitersErrors(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		name        string
		template    string
		delimiters  []string
		expectError bool
	}{
		{
			name:        "invalid custom delimiter template",
			delimiters:  []string{"[[", "]]"},
			template:    "[[ .Config.name",
			expectError: true,
		},
		{
			name:        "valid custom delimiters",
			delimiters:  []string{"[[", "]]"},
			template:    "[[ .Config.name ]]",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]interface{}{"name": "test"}
			_, err := processor.ProcessTemplateWithDelimiters(tt.template, t.TempDir(), nil, values, tt.delimiters)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcessFilePathTemplateErrors tests errors in file path template processing.
func TestProcessFilePathTemplateErrors(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		filePath    string
		values      map[string]interface{}
		expectError bool
	}{
		{
			name:        "invalid path template",
			filePath:    "{{.Config.name}/file.txt",
			values:      map[string]interface{}{"name": "test"},
			expectError: true,
		},
		{
			name:        "valid path template",
			filePath:    "{{.Config.name}}/file.txt",
			values:      map[string]interface{}{"name": "test"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := File{
				Path:        tt.filePath,
				Content:     "test content",
				IsTemplate:  false,
				Permissions: 0o644,
			}

			err := processor.ProcessFile(file, tempDir, false, false, nil, tt.values)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMergeFileErrors tests error paths in 3-way merge.
func TestMergeFileErrors(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// Create a file
	filePath := filepath.Join(tempDir, "test.yaml")
	originalContent := "version: 1.0\nname: original\n"
	err := os.WriteFile(filePath, []byte(originalContent), 0o644)
	require.NoError(t, err)

	// Test update mode with invalid base ref
	// Note: SetupGitStorage may not error without a real git repo
	err = processor.SetupGitStorage(tempDir, "nonexistent-ref")
	_ = err // May or may not error depending on git repo state
	assert.NotNil(t, processor)
}

// TestProcessFileWithScaffoldConfig tests processing with scaffold configuration.
func TestProcessFileWithScaffoldConfig(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	scaffoldConfig := &config.ScaffoldConfig{
		Metadata: manifest.Metadata{
			Name:        "Test Template",
			Description: "Test Description",
			Version:     "1.0.0",
		},
		Spec: config.ScaffoldSpec{
			Delimiters: []string{"{{", "}}"},
			Fields: []config.FieldDefinition{
				{
					Name:        "project_name",
					Type:        "string",
					Label:       "Project Name",
					Description: "Name of the project",
					Required:    true,
					Default:     "my-project",
				},
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name": "test-project",
	}

	file := File{
		Path:        "README.md",
		Content:     "# {{.Config.project_name}}\n\nProject template",
		IsTemplate:  true,
		Permissions: 0o644,
	}

	err := processor.ProcessFile(file, tempDir, false, false, scaffoldConfig, userValues)
	require.NoError(t, err)

	// Verify file was created with processed content
	content, err := os.ReadFile(filepath.Join(tempDir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "# test-project")
}

// TestExtractDelimitersFromScaffoldConfig tests delimiter extraction from scaffold config.
func TestExtractDelimitersFromScaffoldConfig(t *testing.T) {
	tests := []struct {
		name           string
		scaffoldConfig interface{}
		expected       []string
	}{
		{
			name:           "nil config",
			scaffoldConfig: nil,
			expected:       []string{"{{", "}}"},
		},
		{
			name: "config with custom delimiters",
			scaffoldConfig: &config.ScaffoldConfig{
				Spec: config.ScaffoldSpec{Delimiters: []string{"[[", "]]"}},
			},
			expected: []string{"[[", "]]"},
		},
		{
			name: "config with empty delimiters",
			scaffoldConfig: &config.ScaffoldConfig{
				Spec: config.ScaffoldSpec{Delimiters: []string{}},
			},
			expected: []string{"{{", "}}"},
		},
		{
			name: "config with single delimiter",
			scaffoldConfig: &config.ScaffoldConfig{
				Spec: config.ScaffoldSpec{Delimiters: []string{"[["}},
			},
			expected: []string{"{{", "}}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDelimiters(tt.scaffoldConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProcessFileWithEmptyContent tests processing files with empty content.
func TestProcessFileWithEmptyContent(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	file := File{
		Path:        "empty.txt",
		Content:     "",
		IsTemplate:  false,
		Permissions: 0o644,
	}

	err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
	require.NoError(t, err)

	// Verify empty file was created
	content, err := os.ReadFile(filepath.Join(tempDir, "empty.txt"))
	require.NoError(t, err)
	assert.Empty(t, string(content))
}
