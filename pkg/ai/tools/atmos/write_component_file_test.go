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

func TestNewWriteComponentFileTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewWriteComponentFileTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestWriteComponentFileTool_Name(t *testing.T) {
	tool := NewWriteComponentFileTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "write_component_file", tool.Name())
}

func TestWriteComponentFileTool_Description(t *testing.T) {
	tool := NewWriteComponentFileTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Write or modify a file in the components directory")
}

func TestWriteComponentFileTool_Parameters(t *testing.T) {
	tool := NewWriteComponentFileTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	assert.Len(t, params, 3)
	assert.Equal(t, "component_type", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file_path", params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "content", params[2].Name)
	assert.True(t, params[2].Required)
}

func TestWriteComponentFileTool_RequiresPermission(t *testing.T) {
	tool := NewWriteComponentFileTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestWriteComponentFileTool_IsRestricted(t *testing.T) {
	tool := NewWriteComponentFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestWriteComponentFileTool_Execute(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()

	tool := NewWriteComponentFileTool(atmosConfig)
	ctx := context.Background()

	t.Run("successfully writes terraform component file", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/variables.tf",
			"content":        "variable \"cidr_block\" {\n  type = string\n}\n",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "Successfully wrote component file")
		assert.Equal(t, "terraform", result.Data["component_type"])
		assert.Equal(t, "vpc/variables.tf", result.Data["file_path"])

		// Verify file was created.
		filePath := filepath.Join(tmpDir, "components", "terraform", "vpc", "variables.tf")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "variable \"cidr_block\"")

		// Verify permissions.
		info, err := os.Stat(filePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("successfully creates parent directories", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "newdir/nested/test.tf",
			"content":        "# test",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)

		// Verify directory was created.
		dirPath := filepath.Join(tmpDir, "components", "terraform", "newdir", "nested")
		_, err = os.Stat(dirPath)
		require.NoError(t, err)
	})

	t.Run("successfully overwrites existing file", func(t *testing.T) {
		// First write.
		params1 := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/main.tf",
			"content":        "# original",
		}

		result1, err := tool.Execute(ctx, params1)
		require.NoError(t, err)
		assert.True(t, result1.Success)

		// Overwrite.
		params2 := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/main.tf",
			"content":        "# updated",
		}

		result2, err := tool.Execute(ctx, params2)
		require.NoError(t, err)
		assert.True(t, result2.Success)

		// Verify content was overwritten.
		filePath := filepath.Join(tmpDir, "components", "terraform", "vpc", "main.tf")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "# updated", string(content))
	})

	t.Run("fails with missing component_type", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "vpc/test.tf",
			"content":   "test",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"content":        "test",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing content", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/test.tf",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with unsupported component type", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "ansible",
			"file_path":      "test.yml",
			"content":        "test",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIUnsupportedComponentType)
	})

	t.Run("fails with path traversal attempt", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "../../etc/passwd",
			"content":        "malicious",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedComponents)

		// Verify file was NOT created.
		filePath := filepath.Join(tmpDir, "etc", "passwd")
		_, err = os.Stat(filePath)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestWriteFileWithDirs(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("successfully writes file", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "test.txt")
		err := writeFileWithDirs(filePath, "test content")

		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))

		// Check permissions.
		info, err := os.Stat(filePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("successfully creates parent directories", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "a", "b", "c", "test.txt")
		err := writeFileWithDirs(filePath, "nested content")

		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(content))

		// Check directory permissions.
		dirInfo, err := os.Stat(filepath.Join(tmpDir, "a", "b", "c"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm())
	})
}
