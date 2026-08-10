package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func setupListValuesTestProject(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	atmosConfig, _ := setupLiveStackProject(t, map[string]string{
		"dev.yaml": `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        region: us-east-1
`,
		"prod.yaml": `vars:
  stage: prod
components:
  terraform:
    vpc:
      vars:
        region: us-west-2
`,
	})
	return atmosConfig
}

func TestListValuesTool_Interface(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	assert.Equal(t, "atmos_list_values", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 6)
	assert.Equal(t, "component", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "stack", params[1].Name)
	assert.Equal(t, "query", params[2].Name)
	assert.Equal(t, "vars", params[3].Name)
	assert.Equal(t, "include_abstract", params[4].Name)
	assert.Equal(t, "format", params[5].Name)
}

func TestListValuesTool_Execute_MissingComponent(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "component")
}

func TestListValuesTool_Execute_ComponentNotFound(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "does-not-exist",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestListValuesTool_Execute_RegionAcrossStacks(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	// No query: the default output is the full (vars + metadata) section per stack,
	// which includes the `region` var we're comparing across stacks.
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	assert.Contains(t, result.Output, "us-east-1")
	assert.Contains(t, result.Output, "us-west-2")
	assert.Equal(t, "vpc", result.Data["component"])
	assert.Equal(t, "", result.Data["query"])
}

func TestListValuesTool_Execute_VarsShortcut(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
		"vars":      true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	assert.Contains(t, result.Output, "region")
	assert.Equal(t, ".vars", result.Data["query"])
}

func TestListValuesTool_Execute_StackFilter(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
		"stack":     "dev",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	assert.Contains(t, result.Output, "us-east-1")
	assert.NotContains(t, result.Output, "us-west-2")
}

func TestListValuesTool_Execute_JSONFormat(t *testing.T) {
	atmosConfig := setupListValuesTestProject(t)
	tool := NewListValuesTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
		"format":    "json",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	assert.Equal(t, "json", result.Data["format"])
	assert.Contains(t, result.Output, "us-east-1")
}
