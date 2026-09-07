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

func TestNewTerraformComponentHCLEditTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewTerraformComponentHCLEditTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestTerraformComponentHCLEditTool_Name(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_terraform_component_hcl_edit", tool.Name())
}

func TestTerraformComponentHCLEditTool_Description(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestTerraformComponentHCLEditTool_Parameters(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 7)
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	assert.Equal(t, []string{"file_path", "operation", "address", "value", "parent", "child", "newline"}, names)
	assert.True(t, params[0].Required)
	assert.True(t, params[1].Required)
	assert.False(t, params[2].Required)
	assert.False(t, params[3].Required)
	assert.False(t, params[4].Required)
	assert.False(t, params[5].Required)
	assert.False(t, params[6].Required)
	assert.Equal(t, true, params[6].Default)
}

func TestTerraformComponentHCLEditTool_RequiresPermission(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	assert.True(t, tool.RequiresPermission())
}

func TestTerraformComponentHCLEditTool_IsRestricted(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func writeHCLFixture(t *testing.T, tmpDir string) string {
	t.Helper()
	filePath := filepath.Join(tmpDir, "components", "terraform", "vpc", "main.tf")
	require.NoError(t, os.WriteFile(filePath, []byte(hclFixtureWithComments), 0o644))
	return filePath
}

func TestTerraformComponentHCLEditTool_Execute_AttributeSet_PreservesSiblingComment(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	filePath := writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"operation": "attribute_set",
		"address":   "resource.aws_instance.web.instance_type",
		"value":     `"t3.micro"`,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["changed"].(bool))

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "# AMI used for this instance.", "sibling comment untouched")
	assert.Contains(t, s, "t3.micro")
	assert.NotContains(t, s, "t2.micro")
}

func TestTerraformComponentHCLEditTool_Execute_AttributeSet_NotFoundIsNoOp(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"operation": "attribute_set",
		"address":   "resource.aws_instance.web.nonexistent",
		"value":     `"x"`,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.False(t, result.Data["changed"].(bool))
	assert.Contains(t, result.Output, "nothing was changed")
}

func TestTerraformComponentHCLEditTool_Execute_BlockAppend_LifecycleBlock(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	filePath := writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"operation": "block_append",
		"parent":    "resource.aws_instance.web",
		"child":     "lifecycle",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["changed"].(bool))

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "lifecycle")
}

func TestTerraformComponentHCLEditTool_Execute_AttributeRemove_VariableRemoved(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	filePath := writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"operation": "attribute_remove",
		"address":   "variable.region.default",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.True(t, result.Data["changed"].(bool))

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "us-east-1")

	getTool := NewTerraformComponentHCLGetTool(atmosConfig)
	getResult, err := getTool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"address":   "variable.region.default",
	})
	require.Error(t, err)
	assert.False(t, getResult.Success)
}

func TestTerraformComponentHCLEditTool_Execute_MissingFilePath(t *testing.T) {
	tool := NewTerraformComponentHCLEditTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "attribute_set",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
}

func TestTerraformComponentHCLEditTool_Execute_MissingOperation(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
}

func TestTerraformComponentHCLEditTool_Execute_UnknownOperation(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "vpc/main.tf",
		"operation": "bogus_operation",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIUnknownOperation)
}

func TestTerraformComponentHCLEditTool_Execute_MissingRequiredOperationParams(t *testing.T) {
	atmosConfig, tmpDir, cleanup := setupTestComponentEnv(t)
	defer cleanup()
	writeHCLFixture(t, tmpDir)

	tool := NewTerraformComponentHCLEditTool(atmosConfig)

	t.Run("attribute_set without value", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]interface{}{
			"file_path": "vpc/main.tf",
			"operation": "attribute_set",
			"address":   "resource.aws_instance.web.instance_type",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})

	t.Run("block_append without child", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]interface{}{
			"file_path": "vpc/main.tf",
			"operation": "block_append",
			"parent":    "resource.aws_instance.web",
		})
		require.Error(t, err)
		assert.False(t, result.Success)
		assert.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
	})
}

func TestTerraformComponentHCLEditTool_Execute_PathTraversalRejected(t *testing.T) {
	atmosConfig, _, cleanup := setupTestComponentEnv(t)
	defer cleanup()

	tool := NewTerraformComponentHCLEditTool(atmosConfig)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file_path": "../../etc/passwd",
		"operation": "attribute_set",
		"address":   "resource.aws_instance.web.instance_type",
		"value":     `"x"`,
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.ErrorIs(t, err, errUtils.ErrAIFileAccessDeniedComponents)
}

// Note: there is no tool-level test for "edit produces invalid HCL" here.
// hcledit's own filters mutate via hclwrite's AST and are well-behaved by
// design, so there is no way to trigger that outcome through this tool's
// public parameters. The post-edit validity guard's atomicity (never
// persist a broken result) is exercised directly at the pkg/hcl layer -- see
// TestValidateHCL_RejectsInvalidResult in pkg/hcl/edit_test.go.
