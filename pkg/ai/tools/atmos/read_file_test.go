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

func TestReadFileTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewReadFileTool(config)

	assert.Equal(t, "read_file", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 1)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestReadFileTool_Execute_MissingParameter(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewReadFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "file_path")
}

func TestReadFileTool_Execute_FileNotFound(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewReadFileTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "nonexistent.yaml",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestReadFileTool_Execute_PathTraversal(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewReadFileTool(config)
	ctx := context.Background()

	// Try to read file outside base path.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "../../../etc/passwd",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "access denied")
}

func TestReadFileTool_Execute_Success(t *testing.T) {
	// Create temp directory and file.
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")
	testContent := "test: content\nversion: 1.0"
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewReadFileTool(config)
	ctx := context.Background()

	// Test with relative path.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "test.yaml",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, testContent)
	assert.Equal(t, testContent, result.Data["content"])
}

func TestReadFileTool_Execute_Directory(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.Mkdir(subDir, 0o755)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewReadFileTool(config)
	ctx := context.Background()

	// Try to read a directory.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"file_path": "subdir",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "directory")
}
