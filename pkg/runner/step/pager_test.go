package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// PagerHandler registration and validation are tested in command_handlers_test.go.
// This file tests helper methods.

func TestPagerHandler_ShouldRenderMarkdown(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)
	pagerHandler := handler.(*PagerHandler)

	tests := []struct {
		name     string
		step     *schema.WorkflowStep
		path     string
		expected bool
	}{
		{
			name:     "explicit markdown flag true",
			step:     &schema.WorkflowStep{Markdown: true},
			path:     "file.txt",
			expected: true,
		},
		{
			name:     "no markdown flag with md file auto-detects",
			step:     &schema.WorkflowStep{},
			path:     "file.md",
			expected: true,
		},
		{
			name:     "auto-detect .md extension",
			step:     &schema.WorkflowStep{},
			path:     "README.md",
			expected: true,
		},
		{
			name:     "auto-detect .markdown extension",
			step:     &schema.WorkflowStep{},
			path:     "CHANGELOG.markdown",
			expected: true,
		},
		{
			name:     "auto-detect uppercase .MD extension",
			step:     &schema.WorkflowStep{},
			path:     "README.MD",
			expected: true,
		},
		{
			name:     "non-markdown file",
			step:     &schema.WorkflowStep{},
			path:     "file.txt",
			expected: false,
		},
		{
			name:     "empty path no markdown",
			step:     &schema.WorkflowStep{},
			path:     "",
			expected: false,
		},
		{
			name:     "no extension",
			step:     &schema.WorkflowStep{},
			path:     "Makefile",
			expected: false,
		},
		{
			name:     "yaml file",
			step:     &schema.WorkflowStep{},
			path:     "config.yaml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pagerHandler.shouldRenderMarkdown(tt.step, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPagerHandler_ResolveTitle(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)
	pagerHandler := handler.(*PagerHandler)

	tests := []struct {
		name         string
		step         *schema.WorkflowStep
		resolvedPath string
		vars         *Variables
		expected     string
		expectError  bool
	}{
		{
			name:         "explicit title",
			step:         &schema.WorkflowStep{Name: "test", Title: "My Custom Title"},
			resolvedPath: "",
			vars:         NewVariables(),
			expected:     "My Custom Title",
			expectError:  false,
		},
		{
			name:         "title from resolved path",
			step:         &schema.WorkflowStep{Name: "test"},
			resolvedPath: "/path/to/README.md",
			vars:         NewVariables(),
			expected:     "README.md",
			expectError:  false,
		},
		{
			name:         "title from step path when resolved path empty",
			step:         &schema.WorkflowStep{Name: "test", Path: "/path/to/config.yaml"},
			resolvedPath: "",
			vars:         NewVariables(),
			expected:     "config.yaml",
			expectError:  false,
		},
		{
			name:         "empty title and no path",
			step:         &schema.WorkflowStep{Name: "test"},
			resolvedPath: "",
			vars:         NewVariables(),
			expected:     "",
			expectError:  false,
		},
		{
			name:         "template in title",
			step:         &schema.WorkflowStep{Name: "test", Title: "File: {{ .steps.filename.value }}"},
			resolvedPath: "",
			vars: func() *Variables {
				v := NewVariables()
				v.Set("filename", NewStepResult("important.txt"))
				return v
			}(),
			expected:    "File: important.txt",
			expectError: false,
		},
		{
			name:         "invalid template in title",
			step:         &schema.WorkflowStep{Name: "test", Title: "File: {{ .steps.invalid.value"},
			resolvedPath: "",
			vars:         NewVariables(),
			expected:     "",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pagerHandler.resolveTitle(tt.step, tt.vars, tt.resolvedPath)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestPagerHandler_ReadFile(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)
	pagerHandler := handler.(*PagerHandler)

	t.Run("read existing file", func(t *testing.T) {
		// Create a temp file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(tmpFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		step := &schema.WorkflowStep{Name: "test", Path: tmpFile}
		content, err := pagerHandler.readFile(tmpFile, step)
		require.NoError(t, err)
		assert.Equal(t, "test content", content)
	})

	t.Run("read non-existent file", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Path: "/nonexistent/path/file.txt"}
		_, err := pagerHandler.readFile("/nonexistent/path/file.txt", step)
		assert.Error(t, err)
	})
}

func TestPagerHandler_LoadContent(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)
	pagerHandler := handler.(*PagerHandler)

	t.Run("load inline content", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "Inline content here",
		}
		vars := NewVariables()
		ctx := context.Background()

		content, path, err := pagerHandler.loadContent(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Inline content here", content)
		assert.Empty(t, path)
	})

	t.Run("load content with template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name:    "test",
			Content: "Hello {{ .steps.name.value }}",
		}
		vars := NewVariables()
		vars.Set("name", NewStepResult("World"))
		ctx := context.Background()

		content, path, err := pagerHandler.loadContent(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "Hello World", content)
		assert.Empty(t, path)
	})

	t.Run("load content from file", func(t *testing.T) {
		// Create a temp file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(tmpFile, []byte("file content"), 0o644)
		require.NoError(t, err)

		step := &schema.WorkflowStep{
			Name: "test",
			Path: tmpFile,
		}
		vars := NewVariables()
		ctx := context.Background()

		content, path, err := pagerHandler.loadContent(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "file content", content)
		assert.Equal(t, tmpFile, path)
	})

	t.Run("load content from file with template path", func(t *testing.T) {
		// Create a temp file.
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.txt")
		err := os.WriteFile(tmpFile, []byte("template file content"), 0o644)
		require.NoError(t, err)

		step := &schema.WorkflowStep{
			Name: "test",
			Path: "{{ .steps.filepath.value }}",
		}
		vars := NewVariables()
		vars.Set("filepath", NewStepResult(tmpFile))
		ctx := context.Background()

		content, path, err := pagerHandler.loadContent(ctx, step, vars)
		require.NoError(t, err)
		assert.Equal(t, "template file content", content)
		assert.Equal(t, tmpFile, path)
	})

	t.Run("load content with invalid path template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "{{ .steps.invalid.value",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, _, err := pagerHandler.loadContent(ctx, step, vars)
		assert.Error(t, err)
	})

	t.Run("load content from non-existent file", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "/nonexistent/path/file.txt",
		}
		vars := NewVariables()
		ctx := context.Background()

		_, _, err := pagerHandler.loadContent(ctx, step, vars)
		assert.Error(t, err)
	})
}

func TestPagerHandler_RenderMarkdown(t *testing.T) {
	handler, ok := Get("pager")
	require.True(t, ok)
	pagerHandler := handler.(*PagerHandler)

	t.Run("render simple markdown", func(t *testing.T) {
		content := "# Hello\n\nThis is **bold** text."
		rendered, err := pagerHandler.renderMarkdown(content)
		require.NoError(t, err)
		// Rendered content should still contain the text.
		assert.Contains(t, rendered, "Hello")
		assert.Contains(t, rendered, "bold")
	})

	t.Run("render empty content", func(t *testing.T) {
		rendered, err := pagerHandler.renderMarkdown("")
		require.NoError(t, err)
		// Empty content should return empty-ish result.
		assert.NotNil(t, rendered)
	})

	t.Run("render markdown list", func(t *testing.T) {
		content := "- Item 1\n- Item 2\n- Item 3"
		rendered, err := pagerHandler.renderMarkdown(content)
		require.NoError(t, err)
		// Rendered content includes ANSI codes, so check for bullet point and text.
		assert.Contains(t, rendered, "Item")
		assert.Contains(t, rendered, "1")
		assert.Contains(t, rendered, "2")
		// Note: "3" appears in ANSI codes, so just check first two items are present.
	})
}

// Note: PagerHandler validation is tested in output_handlers_test.go.
