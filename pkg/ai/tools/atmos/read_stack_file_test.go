package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupTestStackEnv creates a test environment with stack files.
func setupTestStackEnv(t *testing.T) (*schema.AtmosConfiguration, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create stacks directory.
	stacksDir := filepath.Join(tmpDir, "stacks")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))

	// Create test stack files.
	catalogDir := filepath.Join(stacksDir, "catalog")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))

	vpcStack := `components:
  terraform:
    vpc:
      vars:
        cidr_block: 10.0.0.0/16
`
	require.NoError(t, os.WriteFile(filepath.Join(catalogDir, "vpc.yaml"), []byte(vpcStack), 0o600))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	cleanup := func() {
		// Temp dir auto-cleaned by t.TempDir()
	}

	return atmosConfig, tmpDir, cleanup
}

func TestNewReadStackFileTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewReadStackFileTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestReadStackFileTool_Name(t *testing.T) {
	tool := NewReadStackFileTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "read_stack_file", tool.Name())
}

func TestReadStackFileTool_Description(t *testing.T) {
	tool := NewReadStackFileTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Read a file from the stacks directory")
}

func TestReadStackFileTool_Parameters(t *testing.T) {
	tool := NewReadStackFileTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	assert.Len(t, params, 1)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestReadStackFileTool_RequiresPermission(t *testing.T) {
	tool := NewReadStackFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestReadStackFileTool_IsRestricted(t *testing.T) {
	tool := NewReadStackFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestReadStackFileTool_Execute(t *testing.T) {
	atmosConfig, _, cleanup := setupTestStackEnv(t)
	defer cleanup()

	tool := NewReadStackFileTool(atmosConfig)
	ctx := context.Background()

	t.Run("successfully reads stack file", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog/vpc.yaml",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "cidr_block: 10.0.0.0/16")
		assert.Equal(t, "catalog/vpc.yaml", result.Data["file_path"])
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with empty file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with non-existent file", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog/nonexistent.yaml",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileNotFound)
	})

	t.Run("fails with path traversal attempt", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "../../etc/passwd",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedStacks)
	})

	t.Run("fails when path is a directory", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIPathIsDirectory)
	})
}

func TestExtractFilePathParam(t *testing.T) {
	t.Run("successfully extracts file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog/vpc.yaml",
		}

		filePath, err := extractFilePathParam(params)

		require.NoError(t, err)
		assert.Equal(t, "catalog/vpc.yaml", filePath)
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{}

		_, err := extractFilePathParam(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with empty file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "",
		}

		_, err := extractFilePathParam(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}
