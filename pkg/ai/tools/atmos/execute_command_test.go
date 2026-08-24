package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestExecuteAtmosCommandTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)

	assert.Equal(t, "execute_atmos_command", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.True(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 1)
	assert.Equal(t, "command", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestExecuteAtmosCommandTool_Execute_MissingParameter(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "command")
}

func TestExecuteAtmosCommandTool_Execute_EmptyCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewExecuteAtmosCommandTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestExecuteAtmosCommandTool_Execute_ValidCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewExecuteAtmosCommandTool(config)
	// Override binary to a known command for testing (not the test binary).
	tool.binaryPath = "echo"
	ctx := context.Background()

	// Test with echo which always works.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "hello world",
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "hello world")
}
