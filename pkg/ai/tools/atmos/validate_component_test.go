package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupValidateComponentTestEnv changes into the shared "complete" test fixture (also used by
// internal/exec's ExecuteValidateComponent tests) and returns a fully initialized AtmosConfiguration.
func setupValidateComponentTestEnv(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	t.Chdir("../../../../tests/fixtures/scenarios/complete")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return &atmosConfig
}

func TestValidateComponentTool_Interface(t *testing.T) {
	tool := NewValidateComponentTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_validate_component", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 6)
	assert.Equal(t, "component", params[0].Name)
	assert.Equal(t, "stack", params[1].Name)
	assert.True(t, params[0].Required)
	assert.True(t, params[1].Required)
	assert.False(t, params[2].Required)
}

func TestValidateComponentTool_NewValidateComponentTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewValidateComponentTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestValidateComponentTool_Execute_MissingComponent(t *testing.T) {
	tool := NewValidateComponentTool(&schema.AtmosConfiguration{BasePath: t.TempDir()})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"stack": "tenant1-ue2-dev",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "component")
}

func TestValidateComponentTool_Execute_MissingStack(t *testing.T) {
	tool := NewValidateComponentTool(&schema.AtmosConfiguration{BasePath: t.TempDir()})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "vpc",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "stack")
}

func TestValidateComponentTool_Execute_EmptyComponent(t *testing.T) {
	tool := NewValidateComponentTool(&schema.AtmosConfiguration{BasePath: t.TempDir()})

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "",
		"stack":     "tenant1-ue2-dev",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestValidateComponentTool_Execute_Success(t *testing.T) {
	atmosConfig := setupValidateComponentTestEnv(t)
	tool := NewValidateComponentTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "test/test-component",
		"stack":     "tenant1-ue2-dev",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "test/test-component")
	assert.Contains(t, result.Output, "tenant1-ue2-dev")
	assert.Equal(t, true, result.Data["valid"])
}

func TestValidateComponentTool_Execute_InvalidComponent(t *testing.T) {
	atmosConfig := setupValidateComponentTestEnv(t)
	tool := NewValidateComponentTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "nonexistent-component",
		"stack":     "tenant1-ue2-dev",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestValidateComponentTool_Execute_InvalidStack(t *testing.T) {
	atmosConfig := setupValidateComponentTestEnv(t)
	tool := NewValidateComponentTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component": "test/test-component",
		"stack":     "nonexistent-stack",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestValidateComponentTool_Execute_InvalidSchemaType(t *testing.T) {
	atmosConfig := setupValidateComponentTestEnv(t)
	tool := NewValidateComponentTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component":   "test/test-component",
		"stack":       "tenant1-ue2-dev",
		"schema_path": "test.json",
		"schema_type": "invalid-schema-type",
		"timeout":     float64(30),
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "invalid schema type")
}

func TestValidateComponentTool_Execute_ModulePathsAsInterfaceSlice(t *testing.T) {
	atmosConfig := setupValidateComponentTestEnv(t)
	tool := NewValidateComponentTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"component":    "test/test-component",
		"stack":        "tenant1-ue2-dev",
		"module_paths": []interface{}{"policy1", "policy2"},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
}

func TestExtractModulePathsParam(t *testing.T) {
	assert.Nil(t, extractModulePathsParam(map[string]interface{}{}))
	assert.Equal(t, []string{"a", "b"}, extractModulePathsParam(map[string]interface{}{
		"module_paths": []interface{}{"a", "b"},
	}))
	assert.Equal(t, []string{"a"}, extractModulePathsParam(map[string]interface{}{
		"module_paths": []string{"a"},
	}))
	assert.Nil(t, extractModulePathsParam(map[string]interface{}{
		"module_paths": "not-a-slice",
	}))
}

func TestExtractTimeoutParam(t *testing.T) {
	assert.Equal(t, 0, extractTimeoutParam(map[string]interface{}{}))
	assert.Equal(t, 30, extractTimeoutParam(map[string]interface{}{"timeout": float64(30)}))
	assert.Equal(t, 30, extractTimeoutParam(map[string]interface{}{"timeout": 30}))
	assert.Equal(t, 30, extractTimeoutParam(map[string]interface{}{"timeout": int64(30)}))
	assert.Equal(t, 0, extractTimeoutParam(map[string]interface{}{"timeout": "not-a-number"}))
}
