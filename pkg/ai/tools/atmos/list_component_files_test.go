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

func TestListComponentFilesTool_Execute_HelmfileType(t *testing.T) {
	// Create temp directory structure for helmfile component type.
	tmpDir := t.TempDir()
	helmfileDir := filepath.Join(tmpDir, "components", "helmfile", "charts")
	err := os.MkdirAll(helmfileDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(helmfileDir, "helmfile.yaml"), []byte("releases: []"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "helmfile",
		"component_path": "charts",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "helmfile.yaml")
}

func TestListComponentFilesTool_Execute_PackerType(t *testing.T) {
	// Create temp directory structure for packer component type.
	tmpDir := t.TempDir()
	packerDir := filepath.Join(tmpDir, "components", "packer", "ami")
	err := os.MkdirAll(packerDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(packerDir, "build.pkr.hcl"), []byte("source \"amazon-ebs\" \"main\" {}"), 0o644)
	require.NoError(t, err)

	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	tool := NewListComponentFilesTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component_type": "packer",
		"component_path": "ami",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "build.pkr.hcl")
}

func TestListComponentFilesTool_Execute_PathIsFile(t *testing.T) {
	// Create a file where a directory is expected.
	tmpDir := t.TempDir()
	terraformDir := filepath.Join(tmpDir, "components", "terraform")
	err := os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)

	// Create a file named "vpc" instead of a directory.
	err = os.WriteFile(filepath.Join(terraformDir, "vpc"), []byte("not a dir"), 0o644)
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
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestListComponentFilesTool_Execute_EmptyDirectory(t *testing.T) {
	// Create empty component directory to exercise the "no files found" format path.
	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "components", "terraform", "empty")
	err := os.MkdirAll(emptyDir, 0o755)
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
		"component_path": "empty",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No files found")
}

func TestListComponentFilesTool_Execute_WithFilePatternAndNoMatches(t *testing.T) {
	// Exercise format() with non-wildcard filePattern and no matching files.
	tmpDir := t.TempDir()
	vpcDir := filepath.Join(tmpDir, "components", "terraform", "vpc")
	err := os.MkdirAll(vpcDir, 0o755)
	require.NoError(t, err)

	// Only write a .md file but search for .tf pattern.
	err = os.WriteFile(filepath.Join(vpcDir, "README.md"), []byte("# VPC"), 0o644)
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
		"file_pattern":   "*.tf",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No files found")
}

func TestComponentFileResult_Format_WithPattern(t *testing.T) {
	// Directly test the format method with a non-wildcard pattern to cover header branch.
	r := &componentFileResult{
		files:     []string{"main.tf", "variables.tf"},
		fileCount: 2,
		dirCount:  0,
	}

	output := r.format("terraform", "vpc", "*.tf")

	assert.Contains(t, output, "pattern: *.tf")
	assert.Contains(t, output, "main.tf")
}

func TestComponentFileResult_Format_NoFiles(t *testing.T) {
	// Directly test format() for the empty-results path.
	r := &componentFileResult{}

	output := r.format("terraform", "vpc", "*")

	assert.Contains(t, output, "No files found in terraform/vpc")
}

func TestGetComponentBasePath_AllTypes(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/base",
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
			Helmfile:  schema.Helmfile{BasePath: "components/helmfile"},
			Packer:    schema.Packer{BasePath: "components/packer"},
		},
	}
	tool := NewListComponentFilesTool(config)

	terraformPath, err := tool.getComponentBasePath("terraform")
	require.NoError(t, err)
	assert.Equal(t, "components/terraform", terraformPath)

	helmfilePath, err := tool.getComponentBasePath("helmfile")
	require.NoError(t, err)
	assert.Equal(t, "components/helmfile", helmfilePath)

	packerPath, err := tool.getComponentBasePath("packer")
	require.NoError(t, err)
	assert.Equal(t, "components/packer", packerPath)

	_, err = tool.getComponentBasePath("unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported component type")
}
