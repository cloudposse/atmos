package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditFileTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewEditFileTool(config)

	assert.Equal(t, "edit_file", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.True(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 8)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "operation", params[1].Name)
	assert.True(t, params[1].Required)
}

func TestEditFileTool_Execute_MissingFilePath(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"operation": "append",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "file_path")
}

func TestEditFileTool_Execute_MissingOperation(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.txt",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "operation")
}

func TestEditFileTool_Execute_FileNotFound(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "nonexistent.txt",
		"operation": "append",
		"content":   "test",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "not found")
}

func TestEditFileTool_Execute_SearchReplace(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "version: 1.0.0\nname: test-component\nversion: 1.0.0"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.yaml",
		"operation": "search_replace",
		"search":    "1.0.0",
		"replace":   "2.0.0",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "replaced 2 occurrence(s)")

	// Verify file was modified.
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Contains(t, string(newContent), "version: 2.0.0")
	assert.Contains(t, string(newContent), "name: test-component")
}

func TestEditFileTool_Execute_SearchReplace_NotFound(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "version: 1.0.0\nname: test-component"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.yaml",
		"operation": "search_replace",
		"search":    "nonexistent",
		"replace":   "replacement",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "not found")
}

func TestEditFileTool_Execute_InsertLine(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "line1\nline2\nline3"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path":   "test.yaml",
		"operation":   "insert_line",
		"line_number": float64(2),
		"content":     "inserted_line",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "inserted line at position 2")

	// Verify file was modified.
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	lines := string(newContent)
	assert.Contains(t, lines, "line1\ninserted_line\nline2\nline3")
}

func TestEditFileTool_Execute_InsertLine_InvalidLineNumber(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "line1\nline2"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path":   "test.yaml",
		"operation":   "insert_line",
		"line_number": float64(100),
		"content":     "test",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "out of range")
}

func TestEditFileTool_Execute_DeleteLines(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "line1\nline2\nline3\nline4\nline5"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	// Delete lines 2-4 (inclusive): line2, line3, line4.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path":  "test.yaml",
		"operation":  "delete_lines",
		"start_line": float64(2),
		"end_line":   float64(4),
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "deleted 3 line(s)") // 3 lines: line2, line3, line4

	// Verify file was modified - should only have line1 and line5.
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	lines := string(newContent)
	assert.Contains(t, lines, "line1")
	assert.NotContains(t, lines, "line2")
	assert.NotContains(t, lines, "line3")
	assert.NotContains(t, lines, "line4")
	assert.Contains(t, lines, "line5")
}

func TestEditFileTool_Execute_DeleteLines_InvalidRange(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "line1\nline2"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path":  "test.yaml",
		"operation":  "delete_lines",
		"start_line": float64(5),
		"end_line":   float64(10),
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "invalid line range")
}

func TestEditFileTool_Execute_Append(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "existing content\n"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.yaml",
		"operation": "append",
		"content":   "appended content",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "appended content")

	// Verify file was modified.
	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	assert.Equal(t, "existing content\nappended content", string(newContent))
}

func TestEditFileTool_Execute_UnknownOperation(t *testing.T) {
	// Create a temporary test file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	content := "test"
	require.NoError(t, os.WriteFile(testFile, []byte(content), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.yaml",
		"operation": "unknown_op",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "unknown operation")
}
