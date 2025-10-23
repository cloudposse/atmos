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

func TestNewWriteStackFileTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewWriteStackFileTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestWriteStackFileTool_Name(t *testing.T) {
	tool := NewWriteStackFileTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "write_stack_file", tool.Name())
}

func TestWriteStackFileTool_Description(t *testing.T) {
	tool := NewWriteStackFileTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Write or modify a file in the stacks directory")
}

func TestWriteStackFileTool_Parameters(t *testing.T) {
	tool := NewWriteStackFileTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	assert.Len(t, params, 2)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "content", params[1].Name)
	assert.True(t, params[1].Required)
}

func TestWriteStackFileTool_RequiresPermission(t *testing.T) {
	tool := NewWriteStackFileTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestWriteStackFileTool_IsRestricted(t *testing.T) {
	tool := NewWriteStackFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestWriteStackFileTool_Execute(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestStackEnv(t)
	defer cleanup()

	tool := NewWriteStackFileTool(atmosConfig)
	ctx := context.Background()

	t.Run("successfully writes stack file", func(t *testing.T) {
		stackContent := `components:
  terraform:
    vpc:
      vars:
        cidr_block: 10.1.0.0/16
`
		params := map[string]interface{}{
			"file_path": "catalog/vpc-new.yaml",
			"content":   stackContent,
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "Successfully wrote stack file")
		assert.Equal(t, "catalog/vpc-new.yaml", result.Data["file_path"])

		// Verify file was created.
		filePath := filepath.Join(tmpDir, "stacks", "catalog", "vpc-new.yaml")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "cidr_block: 10.1.0.0/16")

		// Verify permissions.
		info, err := os.Stat(filePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("successfully creates parent directories", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "orgs/acme/dev/networking.yaml",
			"content":   "# new stack",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify directory was created.
		dirPath := filepath.Join(tmpDir, "stacks", "orgs", "acme", "dev")
		_, err = os.Stat(dirPath)
		require.NoError(t, err)
	})

	t.Run("successfully overwrites existing file", func(t *testing.T) {
		// First write.
		params1 := map[string]interface{}{
			"file_path": "catalog/vpc.yaml",
			"content":   "# original",
		}

		result1, err := tool.Execute(ctx, params1)
		require.NoError(t, err)
		assert.True(t, result1.Success)

		// Overwrite.
		params2 := map[string]interface{}{
			"file_path": "catalog/vpc.yaml",
			"content":   "# updated",
		}

		result2, err := tool.Execute(ctx, params2)
		require.NoError(t, err)
		assert.True(t, result2.Success)

		// Verify content was overwritten.
		filePath := filepath.Join(tmpDir, "stacks", "catalog", "vpc.yaml")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "# updated", string(content))
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"content": "test",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with empty file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "",
			"content":   "test",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing content", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog/test.yaml",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with path traversal attempt", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "../../etc/passwd",
			"content":   "malicious",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedStacks)

		// Verify file was NOT created.
		filePath := filepath.Join(tmpDir, "etc", "passwd")
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("handles empty content", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "catalog/empty.yaml",
			"content":   "",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify empty file was created.
		filePath := filepath.Join(tmpDir, "stacks", "catalog", "empty.yaml")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "", string(content))
	})
}
