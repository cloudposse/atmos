package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeComponentTool_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	config := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	tool := NewDescribeComponentTool(config)

	assert.Equal(t, "atmos_describe_component", tool.Name())
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

func TestDescribeComponentTool_Execute_MissingComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"stack": "tenant1-ue2-dev",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "component")
}

func TestDescribeComponentTool_Execute_EmptyComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "",
		"stack":     "tenant1-ue2-dev",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "component")
}

func TestDescribeComponentTool_Execute_MissingStack(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "stack")
}

func TestDescribeComponentTool_Execute_EmptyStack(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
		"stack":     "",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "stack")
}

func TestDescribeComponentTool_Execute_InvalidParameterType(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	// Component as non-string type.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": 123,
		"stack":     "tenant1-ue2-dev",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "component")
}

func TestDescribeComponentTool_Execute_InvalidStackType(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	// Stack as non-string type.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
		"stack":     []string{"tenant1-ue2-dev"},
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
	assert.Contains(t, err.Error(), "stack")
}

func TestDescribeComponentTool_Execute_InvalidComponent(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "nonexistent-component",
		"stack":     "plat-ue2-prod",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_Execute_InvalidStack(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
		"stack":     "nonexistent-stack",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_Execute_Success(t *testing.T) {
	t.Skip("Integration test requires real stack files - skipped for unit tests")

	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
		"stack":     "plat-ue2-prod",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.Output)
	assert.Contains(t, result.Output, "Component: vpc")
	assert.Contains(t, result.Output, "Stack: plat-ue2-prod")
	assert.Contains(t, result.Output, "Configuration:")

	// Check data structure.
	assert.NotNil(t, result.Data)
	assert.Equal(t, "vpc", result.Data["component"])
	assert.Equal(t, "plat-ue2-prod", result.Data["stack"])
	assert.NotNil(t, result.Data["config"])

	// Verify config is a map.
	configData, ok := result.Data["config"].(map[string]any)
	assert.True(t, ok)
	assert.NotEmpty(t, configData)
}

func TestDescribeComponentTool_Execute_OutputFormat(t *testing.T) {
	t.Skip("Integration test requires real stack files - skipped for unit tests")

	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "vpc",
		"stack":     "plat-ue2-prod",
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)

	// Verify YAML format in output.
	assert.Contains(t, result.Output, "Component: vpc")
	assert.Contains(t, result.Output, "Stack: plat-ue2-prod")
	assert.Contains(t, result.Output, "Configuration:")

	// Output should be YAML formatted (contains key: value pairs).
	// YAML typically uses colons and proper indentation.
	output := result.Output
	assert.NotContains(t, output, "map[", "Output should not contain Go map representation")
	assert.Contains(t, output, ":", "Output should contain YAML key-value separator")
}

func TestDescribeComponentTool_Execute_EmptyParams(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_Execute_NilParams(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	result, err := tool.Execute(ctx, nil)

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_Execute_ExtraParams(t *testing.T) {
	t.Skip("Integration test requires real stack files - skipped for unit tests")

	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	// Extra parameters should be ignored.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component":    "vpc",
		"stack":        "plat-ue2-prod",
		"extra_param1": "value1",
		"extra_param2": 123,
	})

	assert.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.Output)
}

func TestDescribeComponentTool_Execute_CaseSensitivity(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../examples/quick-start-advanced",
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	// Component names are case-sensitive.
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "VPC",
		"stack":     "plat-ue2-prod",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_Execute_WhitespaceInParameters(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)
	ctx := context.Background()

	// Component with whitespace should be treated as-is (not trimmed by tool).
	result, err := tool.Execute(ctx, map[string]interface{}{
		"component": "  vpc  ",
		"stack":     "plat-ue2-prod",
	})

	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Error(t, result.Error)
}

func TestDescribeComponentTool_NewDescribeComponentTool(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeComponentTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestDescribeComponentTool_NewDescribeComponentTool_NilConfig(t *testing.T) {
	tool := NewDescribeComponentTool(nil)

	assert.NotNil(t, tool)
	assert.Nil(t, tool.atmosConfig)
}
