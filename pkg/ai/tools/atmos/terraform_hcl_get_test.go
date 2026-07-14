package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	hcl "github.com/cloudposse/atmos/pkg/hcl"
	"github.com/cloudposse/atmos/pkg/schema"
)

const hclFixtureWithComments = `# Header comment.
resource "aws_instance" "web" {
  # AMI used for this instance.
  ami           = "ami-123456"
  instance_type = "t2.micro" # inline comment
}

variable "region" {
  description = "AWS region"
  default     = "us-east-1"
}
`

func TestNewTerraformComponentHCLGetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewTerraformComponentHCLGetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestTerraformComponentHCLGetTool_Name(t *testing.T) {
	tool := NewTerraformComponentHCLGetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_terraform_component_hcl_get", tool.Name())
}

func TestTerraformComponentHCLGetTool_Description(t *testing.T) {
	tool := NewTerraformComponentHCLGetTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestTerraformComponentHCLGetTool_Parameters(t *testing.T) {
	tool := NewTerraformComponentHCLGetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 3)
	assert.Equal(t, "file_path", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "address", params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "with_comments", params[2].Name)
	assert.False(t, params[2].Required)
	assert.Equal(t, false, params[2].Default)
}

func TestTerraformComponentHCLGetTool_RequiresPermission(t *testing.T) {
	tool := NewTerraformComponentHCLGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestTerraformComponentHCLGetTool_IsRestricted(t *testing.T) {
	tool := NewTerraformComponentHCLGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestTerraformComponentHCLGetTool_Execute(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()

	filePath := filepath.Join(tmpDir, "components", "terraform", "vpc", "main.tf")
	require.NoError(t, os.WriteFile(filePath, []byte(hclFixtureWithComments), 0o644))

	tool := NewTerraformComponentHCLGetTool(atmosConfig)
	ctx := context.Background()

	t.Run("gets an attribute", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "vpc/main.tf",
			"address":   "resource.aws_instance.web.instance_type",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Equal(t, `"t2.micro"`, result.Data["value"])
	})

	t.Run("gets an attribute with comments", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path":     "vpc/main.tf",
			"address":       "resource.aws_instance.web.instance_type",
			"with_comments": true,
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Data["value"], "inline comment")
	})

	t.Run("falls back to a block", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "vpc/main.tf",
			"address":   "resource.aws_instance.web",
		})
		require.NoError(t, err)
		assert.True(t, result.Success)
		assert.Contains(t, result.Data["value"], "ami-123456")
	})

	t.Run("fails when address is not found", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "vpc/main.tf",
			"address":   "resource.aws_instance.nonexistent.foo",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, hcl.ErrHCLAddressNotFound)
	})

	t.Run("fails with missing file_path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"address": "resource.aws_instance.web.instance_type",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with missing address", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "vpc/main.tf",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("fails with path traversal attempt", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "../../etc/passwd",
			"address":   "resource.aws_instance.web.instance_type",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedComponents)
	})

	t.Run("fails with nonexistent file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"file_path": "vpc/nonexistent.tf",
			"address":   "resource.aws_instance.web.instance_type",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIFileNotFound)
	})
}
