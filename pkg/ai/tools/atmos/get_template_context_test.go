package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestGetTemplateContextTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewGetTemplateContextTool(config)

	assert.Equal(t, "get_template_context", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 2)
	assert.Equal(t, "component", params[0].Name)
	assert.Equal(t, "stack", params[1].Name)
	assert.True(t, params[0].Required)
	assert.True(t, params[1].Required)
}

func TestGetTemplateContextTool_Execute_MissingComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewGetTemplateContextTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"stack": "prod-us-east-1",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "component")
}

func TestGetTemplateContextTool_Execute_MissingStack(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewGetTemplateContextTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "stack")
}

func TestGetTemplateContextTool_Execute_InvalidComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced", // Use real example.
	}

	tool := NewGetTemplateContextTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "nonexistent-component",
		"stack":     "tenant1-ue2-dev",
	})

	assert.NoError(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}
