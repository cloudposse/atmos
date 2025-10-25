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

func TestListComponentFilesTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tool := NewListComponentFilesTool(config)

	assert.Equal(t, "list_component_files", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 3)
	assert.Equal(t, "component_type", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestListComponentFilesTool_Execute_MissingComponentType(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "component_type")
}

func TestListComponentFilesTool_Execute_InvalidComponentType(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "invalid",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestListComponentFilesTool_Execute_Success(t *testing.T) {
	// Create temp directory structure.
	tmpDir := t.TempDir()
	terraformDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	err := os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)

	// Create test files.
	files := []string{"main.tf", "variables.tf", "outputs.tf", "README.md"}
	for _, file := range files {
		err := os.WriteFile(filepath.Join(terraformDir, file), []byte("test"), 0o644)
		require.NoError(t, err)
	}

	// Create subdirectory.
	subDir := filepath.Join(terraformDir, "modules")
	err = os.Mkdir(subDir, 0o755)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "terraform",
		"component_path": "vpc",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "main.tf")
	assert.Contains(t, result.Output, "variables.tf")
	assert.Equal(t, 4, result.Data["file_count"])
	assert.Equal(t, 1, result.Data["dir_count"])
}

func TestListComponentFilesTool_Execute_WithFilePattern(t *testing.T) {
	// Create temp directory structure.
	tmpDir := t.TempDir()
	terraformDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	err := os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)

	// Create test files.
	files := map[string]string{
		"main.tf":      "test",
		"variables.tf": "test",
		"README.md":    "test",
	}
	for file, content := range files {
		err := os.WriteFile(filepath.Join(terraformDir, file), []byte(content), 0o644)
		require.NoError(t, err)
	}

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	// List only .tf files.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "terraform",
		"component_path": "vpc",
		"file_pattern":   "*.tf",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "main.tf")
	assert.NotContains(t, result.Output, "README.md")
	assert.Equal(t, 2, result.Data["file_count"])
}

func TestListComponentFilesTool_Execute_ComponentNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	terraformDir := filepath.Join(tmpDir, "components", "terraform")
	err := os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "terraform",
		"component_path": "nonexistent",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}
