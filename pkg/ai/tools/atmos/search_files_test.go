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

func TestSearchFilesTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewSearchFilesTool(config)

	assert.Equal(t, "search_files", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 4)
	assert.Equal(t, "pattern", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestSearchFilesTool_Execute_MissingPattern(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "pattern")
}

func TestSearchFilesTool_Execute_InvalidPattern(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "[invalid(",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "invalid pattern")
}

func TestSearchFilesTool_Execute_Success(t *testing.T) {
	// Create temp directory with test files.
	tmpDir := t.TempDir()

	// Create test files.
	file1 := filepath.Join(tmpDir, "test1.yaml")
	err := os.WriteFile(file1, []byte("backend_type: s3\nregion: us-east-1"), 0o644)
	require.NoError(t, err)

	file2 := filepath.Join(tmpDir, "test2.yaml")
	err = os.WriteFile(file2, []byte("backend_type: s3\nregion: us-west-2"), 0o644)
	require.NoError(t, err)

	file3 := filepath.Join(tmpDir, "test3.txt")
	err = os.WriteFile(file3, []byte("no match here"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	// Search for pattern.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "backend_type",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "backend_type")
	assert.Equal(t, 2, result.Data["match_count"])
	assert.Equal(t, 2, result.Data["file_count"])
}

func TestSearchFilesTool_Execute_WithFilePattern(t *testing.T) {
	// Create temp directory with test files.
	tmpDir := t.TempDir()

	// Create test files.
	file1 := filepath.Join(tmpDir, "test1.yaml")
	err := os.WriteFile(file1, []byte("test: value"), 0o644)
	require.NoError(t, err)

	file2 := filepath.Join(tmpDir, "test2.txt")
	err = os.WriteFile(file2, []byte("test: value"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	// Search only in YAML files.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern":      "test",
		"file_pattern": "*.yaml",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.Data["file_count"])
}

func TestSearchFilesTool_Execute_CaseSensitive(t *testing.T) {
	// Create temp directory with test file.
	tmpDir := t.TempDir()

	file := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(file, []byte("TEST: value\ntest: value"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	// Case-insensitive search (default).
	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "TEST",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.Data["match_count"])

	// Case-sensitive search.
	result, err = tool.Execute(ctx, map[string]interface{}{
		"pattern":        "TEST",
		"case_sensitive": true,
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 1, result.Data["match_count"])
}

func TestSearchFilesTool_Execute_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	file := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(file, []byte("test: value"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "nonexistent",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No matches found")
	assert.Equal(t, 0, result.Data["match_count"])
}
