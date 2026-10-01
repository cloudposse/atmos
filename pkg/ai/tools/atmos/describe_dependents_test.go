package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeDependentsTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeDependentsTool(config)

	assert.Equal(t, "atmos_describe_dependents", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 3)
	assert.Equal(t, "component", params[0].Name)
	assert.True(t, params[0].Required)
	assert.Equal(t, "stack", params[1].Name)
	assert.True(t, params[1].Required)
	assert.Equal(t, "include_settings", params[2].Name)
	assert.False(t, params[2].Required)
}

func TestDescribeDependentsTool_Execute_MissingComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeDependentsTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"stack": "dev",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "component")
}

func TestDescribeDependentsTool_Execute_MissingStack(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeDependentsTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "stack")
}

func TestDescribeDependentsTool_Execute_FindsDependents(t *testing.T) {
	atmosConfig, _ := setupLiveStackProject(t, map[string]string{
		"dev.yaml": `vars:
  stage: dev
components:
  terraform:
    vpc:
      metadata:
        component: vpc
      vars: {}
    tgw:
      metadata:
        component: tgw
      vars: {}
      settings:
        depends_on:
          1:
            component: vpc
`,
	})

	tool := NewDescribeDependentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
		"stack":     "dev",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 1, count)

	dependents, ok := result.Data["dependents"].([]schema.Dependent)
	require.True(t, ok)
	require.Len(t, dependents, 1)
	assert.Equal(t, "tgw", dependents[0].Component)
	assert.Equal(t, "dev", dependents[0].Stack)

	assert.Contains(t, result.Output, "tgw")
}

func TestDescribeDependentsTool_Execute_NoDependents(t *testing.T) {
	atmosConfig, _ := setupLiveStackProject(t, map[string]string{
		"dev.yaml": `vars:
  stage: dev
components:
  terraform:
    vpc:
      metadata:
        component: vpc
      vars: {}
`,
	})

	tool := NewDescribeDependentsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
		"stack":     "dev",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 0, count)
}
