package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewStackConfigGetTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewStackConfigGetTool(atmosConfig)

	assert.NotNil(t, tool)
	assert.Same(t, atmosConfig, tool.atmosConfig)
}

func TestStackConfigGetTool_Name(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_stack_config_get", tool.Name())
}

func TestStackConfigGetTool_Description(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	assert.NotEmpty(t, tool.Description())
}

func TestStackConfigGetTool_Parameters(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 4)
	assert.Equal(t, paramStack, params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, paramComponent, params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "path", params[2].Name)
	assert.True(t, params[2].Required)
	assert.Equal(t, "file", params[3].Name)
	assert.False(t, params[3].Required)
}

func TestStackConfigGetTool_RequiresPermission(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestStackConfigGetTool_IsRestricted(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestStackConfigGetTool_Execute_MissingStack(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramComponent: "vpc",
		"path":         "vars.region",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), paramStack)
}

func TestStackConfigGetTool_Execute_MissingComponent(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack: "dev",
		"path":     "vars.region",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), paramComponent)
}

func TestStackConfigGetTool_Execute_MissingPath(t *testing.T) {
	tool := NewStackConfigGetTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "path")
}

func TestStackConfigGetTool_Execute_ProvenanceOverride(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigGetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.foo",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, "dev-override", result.Data["value"])
	assert.Equal(t, "dev.yaml", result.Data["file"])
	assert.Contains(t, result.Output, "vars.foo")
	assert.Contains(t, result.Output, "dev-override")
}

func TestStackConfigGetTool_Execute_ExplicitFile(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigGetTool(atmosConfig)

	dir := t.TempDir()
	file := filepath.Join(dir, "explicit.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`components:
  terraform:
    vpc:
      vars:
        region: eu-west-1
`), 0o644))

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.region",
		"file":         file,
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Equal(t, "eu-west-1", result.Data["value"])
}

func TestStackConfigGetTool_Execute_UndefinedPath(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigGetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "dev",
		paramComponent: "vpc",
		"path":         "vars.does_not_exist",
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	assert.Empty(t, result.Data["value"])
}

func TestStackConfigGetTool_Execute_InvalidStack(t *testing.T) {
	atmosConfig, _ := stackConfigLiveFixture(t)
	tool := NewStackConfigGetTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		paramStack:     "does-not-exist",
		paramComponent: "vpc",
		"path":         "vars.region",
	})
	require.Error(t, err)
	assert.False(t, result.Success)
}
