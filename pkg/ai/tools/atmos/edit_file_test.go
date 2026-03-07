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

func TestEditFileTool_Execute_PathOutsideBase(t *testing.T) {
	// Create two separate temp directories so path traversal can be tested.
	tmpDir := t.TempDir()

	config := &schema.AtmosConfiguration{
		BasePath: filepath.Join(tmpDir, "project"),
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	// Attempt to edit a file outside the base path using absolute path.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": tmpDir, // parent dir, outside base
		"operation": "append",
		"content":   "test",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "access denied")
}

func TestEditFileTool_ExtractEditParams_MissingFilePath(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp",
	}
	tool := NewEditFileTool(config)

	ep, errResult := tool.extractEditParams(map[string]interface{}{
		"operation": "append",
	})

	assert.Nil(t, ep)
	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "file_path")
}

func TestEditFileTool_ExtractEditParams_MissingOperation(t *testing.T) {
	tmpDir := t.TempDir()
	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}
	tool := NewEditFileTool(config)

	ep, errResult := tool.extractEditParams(map[string]interface{}{
		"file_path": "test.txt",
	})

	assert.Nil(t, ep)
	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "operation")
}

func TestEditFileTool_ExtractEditParams_EmptyOperation(t *testing.T) {
	tmpDir := t.TempDir()
	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}
	tool := NewEditFileTool(config)

	ep, errResult := tool.extractEditParams(map[string]interface{}{
		"file_path": "test.txt",
		"operation": "",
	})

	assert.Nil(t, ep)
	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "operation")
}

func TestReadFileForEdit_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "ghost.txt")

	content, errResult := readFileForEdit(nonExistent, "ghost.txt")

	assert.Nil(t, content)
	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "not found")
}

func TestReadFileForEdit_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "present.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("hello"), 0o600))

	content, errResult := readFileForEdit(testFile, "present.txt")

	assert.Nil(t, errResult)
	require.NotNil(t, content)
	assert.Equal(t, "hello", string(content))
}

func TestEditFileTool_SearchReplace_MissingSearch(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.searchReplace("content", map[string]interface{}{
		"replace": "new",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "search")
}

func TestEditFileTool_SearchReplace_MissingReplace(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.searchReplace("content", map[string]interface{}{
		"search": "content",
		// "replace" key absent — not a string assertion
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "replace")
}

func TestEditFileTool_InsertLine_MissingLineNumber(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.insertLine("line1\nline2", map[string]interface{}{
		"content": "new line",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line_number")
}

func TestEditFileTool_InsertLine_MissingContent(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.insertLine("line1\nline2", map[string]interface{}{
		"line_number": float64(1),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "content")
}

func TestEditFileTool_InsertLine_NegativeLineNumber(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.insertLine("line1\nline2", map[string]interface{}{
		"line_number": float64(0), // 0 converts to index -1
		"content":     "inserted",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestEditFileTool_DeleteLines_MissingStartLine(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.deleteLines("line1\nline2\nline3", map[string]interface{}{
		"end_line": float64(2),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "start_line")
}

func TestEditFileTool_DeleteLines_MissingEndLine(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.deleteLines("line1\nline2\nline3", map[string]interface{}{
		"start_line": float64(1),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "end_line")
}

func TestEditFileTool_DeleteLines_StartAfterEnd(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.deleteLines("line1\nline2\nline3", map[string]interface{}{
		"start_line": float64(3),
		"end_line":   float64(1),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid line range")
}

func TestEditFileTool_AppendContent_MissingContent(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: "/tmp"}
	tool := NewEditFileTool(config)

	_, _, err := tool.appendContent("existing\n", map[string]interface{}{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "content")
}

func TestEditFileTool_Execute_Append_NoTrailingNewline(t *testing.T) {
	// Test appendContent when file does NOT end with newline.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	// File content without trailing newline.
	require.NoError(t, os.WriteFile(testFile, []byte("existing content"), 0o600))

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewEditFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.txt",
		"operation": "append",
		"content":   "appended",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)

	newContent, err := os.ReadFile(testFile)
	require.NoError(t, err)
	// A newline should have been inserted before the appended content.
	assert.Equal(t, "existing content\nappended", string(newContent))
}
