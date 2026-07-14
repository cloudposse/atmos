package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func setupListComponentsTestProject(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	atmosConfig, _ := setupLiveStackProject(t, map[string]string{
		"dev.yaml": `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars: {}
    tgw-mixin:
      metadata:
        type: abstract
      vars: {}
`,
		"prod.yaml": `vars:
  stage: prod
components:
  terraform:
    vpc:
      vars: {}
`,
	})
	return atmosConfig
}

func TestListComponentsTool_Interface(t *testing.T) {
	atmosConfig := setupListComponentsTestProject(t)
	tool := NewListComponentsTool(atmosConfig)

	assert.Equal(t, "atmos_list_components", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 3)
	assert.Equal(t, "stack", params[0].Name)
	assert.Equal(t, "type", params[1].Name)
	assert.Equal(t, "include_abstract", params[2].Name)
	assert.False(t, params[0].Required)
	assert.False(t, params[1].Required)
	assert.False(t, params[2].Required)
}

func TestListComponentsTool_Execute_DefaultExcludesAbstract(t *testing.T) {
	atmosConfig := setupListComponentsTestProject(t)
	tool := NewListComponentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	components, ok := result.Data["components"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, components, 1)
	assert.Equal(t, "vpc", components[0]["component"])
	assert.Equal(t, 2, components[0]["stack_count"])

	assert.Contains(t, result.Output, "vpc")
	assert.NotContains(t, result.Output, "tgw-mixin")
}

func TestListComponentsTool_Execute_IncludeAbstract(t *testing.T) {
	atmosConfig := setupListComponentsTestProject(t)
	tool := NewListComponentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"include_abstract": true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	components, ok := result.Data["components"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, components, 2)

	names := []string{}
	for _, c := range components {
		name, _ := c["component"].(string)
		names = append(names, name)
	}
	assert.Contains(t, names, "vpc")
	assert.Contains(t, names, "tgw-mixin")
}

func TestListComponentsTool_Execute_StackFilter(t *testing.T) {
	atmosConfig := setupListComponentsTestProject(t)
	tool := NewListComponentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"stack":            "dev",
		"include_abstract": true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	components, ok := result.Data["components"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, components, 2)
	for _, c := range components {
		assert.Equal(t, 1, c["stack_count"])
	}
}

func TestListComponentsTool_Execute_TypeFilterExcludesAll(t *testing.T) {
	atmosConfig := setupListComponentsTestProject(t)
	tool := NewListComponentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type": "helmfile",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	components, ok := result.Data["components"].([]map[string]any)
	require.True(t, ok)
	assert.Empty(t, components)
}
