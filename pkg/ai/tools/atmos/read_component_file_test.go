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

// setupTestComponentEnv creates a test environment with component files.
func setupTestComponentEnv(t *testing.T) (*schema.AtmosConfiguration, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create component directories.
	terraformDir := filepath.Join(tmpDir, "components", "terraform")
	helmfileDir := filepath.Join(tmpDir, "components", "helmfile")
	packerDir := filepath.Join(tmpDir, "components", "packer")

	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	require.NoError(t, os.MkdirAll(helmfileDir, 0o755))
	require.NoError(t, os.MkdirAll(packerDir, 0o755))

	// Create test files.
	vpcDir := filepath.Join(terraformDir, "vpc")
	require.NoError(t, os.MkdirAll(vpcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vpcDir, "main.tf"), []byte("resource \"aws_vpc\" \"main\" {}"), 0o600))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	cleanup := func() {
		// Temp dir auto-cleaned by t.TempDir()
	}

	return atmosConfig, tmpDir, cleanup
}

func TestNewReadComponentFileTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewReadComponentFileTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestReadComponentFileTool_Name(t *testing.T) {
	tool := NewReadComponentFileTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "read_component_file", tool.Name())
}

func TestReadComponentFileTool_Description(t *testing.T) {
	tool := NewReadComponentFileTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Read a file from the components directory")
}

func TestReadComponentFileTool_Parameters(t *testing.T) {
	tool := NewReadComponentFileTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	assert.Len(t, params, 2)
	assert.Equal(t, "component_type", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "file_path", params[1].Name)
	assert.True(t, params[1].Required)
}

func TestReadComponentFileTool_RequiresPermission(t *testing.T) {
	tool := NewReadComponentFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestReadComponentFileTool_IsRestricted(t *testing.T) {
	tool := NewReadComponentFileTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestReadComponentFileTool_Execute(t *testing.T) {
	atmosConfig, _, cleanup := setupTestComponentEnv(t)
	defer cleanup()

	tool := NewReadComponentFileTool(atmosConfig)
	ctx := context.Background()

	t.Run("successfully reads terraform component file", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/main.tf",
		}

		result, err := tool.Execute(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Output, "resource \"aws_vpc\" \"main\"")
		assert.Equal(t, "vpc/main.tf", result.Data["file_path"])
		assert.Equal(t, "terraform", result.Data["component_type"])
	})

	t.Run("fails with missing component_type", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "vpc/main.tf",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
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
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIUnsupportedComponentType)
	})

	t.Run("fails with non-existent file", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/nonexistent.tf",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileNotFound)
	})

	t.Run("fails with path traversal attempt", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "../../etc/passwd",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedComponents)
	})

	t.Run("fails when path is a directory", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc",
		}

		result, err := tool.Execute(ctx, params)

		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIPathIsDirectory)
	})
}

func TestExtractComponentParams(t *testing.T) {
	t.Run("successfully extracts valid params", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
			"file_path":      "vpc/main.tf",
		}

		componentType, filePath, err := extractComponentParams(params)

		require.NoError(t, err)
		assert.Equal(t, "terraform", componentType)
		assert.Equal(t, "vpc/main.tf", filePath)
	})

	t.Run("fails with missing component_type", func(t *testing.T) {
		params := map[string]interface{}{
			"file_path": "vpc/main.tf",
		}

		_, _, err := extractComponentParams(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with empty component_type", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "",
			"file_path":      "vpc/main.tf",
		}

		_, _, err := extractComponentParams(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		params := map[string]interface{}{
			"component_type": "terraform",
		}

		_, _, err := extractComponentParams(params)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}

func TestReadAndValidateFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("successfully reads file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o600))

		content, err := readAndValidateFile(testFile, "test.txt")

		require.NoError(t, err)
		assert.Equal(t, "test content", string(content))
	})

	t.Run("fails with non-existent file", func(t *testing.T) {
		_, err := readAndValidateFile(filepath.Join(tmpDir, "nonexistent.txt"), "nonexistent.txt")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIFileNotFound)
	})

	t.Run("fails with directory", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "testdir")
		require.NoError(t, os.MkdirAll(testDir, 0o755))

		_, err := readAndValidateFile(testDir, "testdir")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIPathIsDirectory)
	})
}
