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

func TestSearchFilesTool_Execute_WithDotPath(t *testing.T) {
	// Test explicit path="." to ensure base path access works.
	tmpDir := t.TempDir()

	file := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(file, []byte("test_pattern: value"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	// Explicitly pass path=".".
	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "test_pattern",
		"path":    ".",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "test_pattern")
	assert.Equal(t, 1, result.Data["match_count"])
}

func TestSearchFilesTool_Execute_PathOutsideBase(t *testing.T) {
	// Create two separate temp directories so path traversal can be tested.
	tmpDir := t.TempDir()

	config := &schema.AtmosConfiguration{
		BasePath: filepath.Join(tmpDir, "project"),
	}

	tool := NewSearchFilesTool(config)
	ctx := context.Background()

	// Request a search path that resolves outside BasePath.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"pattern": "anything",
		"path":    "../../outside",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "access denied")
}

func TestExtractSearchParams_Defaults(t *testing.T) {
	// Only the required "pattern" key — verify defaults are applied.
	sp, errResult := extractSearchParams(map[string]interface{}{
		"pattern": "mypattern",
	})

	require.Nil(t, errResult)
	require.NotNil(t, sp)
	assert.Equal(t, "mypattern", sp.pattern)
	assert.Equal(t, ".", sp.searchPath)
	assert.Equal(t, "*", sp.filePattern)
	assert.False(t, sp.caseSensitive)
}

func TestExtractSearchParams_WithAllParams(t *testing.T) {
	sp, errResult := extractSearchParams(map[string]interface{}{
		"pattern":        "vpc",
		"path":           "stacks",
		"file_pattern":   "*.yaml",
		"case_sensitive": true,
	})

	require.Nil(t, errResult)
	require.NotNil(t, sp)
	assert.Equal(t, "vpc", sp.pattern)
	assert.Equal(t, "stacks", sp.searchPath)
	assert.Equal(t, "*.yaml", sp.filePattern)
	assert.True(t, sp.caseSensitive)
}

func TestExtractSearchParams_EmptyPattern(t *testing.T) {
	_, errResult := extractSearchParams(map[string]interface{}{
		"pattern": "",
	})

	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "pattern")
}

func TestExtractSearchParams_MissingPattern(t *testing.T) {
	_, errResult := extractSearchParams(map[string]interface{}{})

	require.NotNil(t, errResult)
	assert.False(t, errResult.Success)
	assert.Contains(t, errResult.Error.Error(), "pattern")
}

func TestBuildSearchResult_WithMatches(t *testing.T) {
	sp := &searchParams{
		pattern:     "backend_type",
		searchPath:  "stacks",
		filePattern: "*.yaml",
	}
	matches := []string{"\nstacks/dev.yaml:\n  Line 3: backend_type: s3"}
	matchCount := 1

	result := buildSearchResult(sp, matches, matchCount)

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Found 1 matches in 1 files")
	assert.Equal(t, "backend_type", result.Data["pattern"])
	assert.Equal(t, "stacks", result.Data["path"])
	assert.Equal(t, "*.yaml", result.Data["file_pattern"])
	assert.Equal(t, 1, result.Data["match_count"])
	assert.Equal(t, 1, result.Data["file_count"])
}

func TestBuildSearchResult_NoMatches(t *testing.T) {
	sp := &searchParams{
		pattern:     "nonexistent",
		searchPath:  ".",
		filePattern: "*",
	}

	result := buildSearchResult(sp, nil, 0)

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No matches found for pattern 'nonexistent'")
	assert.Equal(t, 0, result.Data["match_count"])
	assert.Equal(t, 0, result.Data["file_count"])
}
