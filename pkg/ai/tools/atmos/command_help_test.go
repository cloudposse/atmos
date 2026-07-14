package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewCommandHelpTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewCommandHelpTool(atmosConfig)

	require.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestCommandHelpTool_Name(t *testing.T) {
	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_command_help", tool.Name())
}

func TestCommandHelpTool_Description(t *testing.T) {
	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "Get detailed help")
}

func TestCommandHelpTool_Parameters(t *testing.T) {
	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 1)
	assert.Equal(t, "command", params[0].Name)
	assert.True(t, params[0].Required)
}

func TestCommandHelpTool_RequiresPermission(t *testing.T) {
	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestCommandHelpTool_IsRestricted(t *testing.T) {
	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestCommandHelpTool_Execute_MissingCommandParam(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})

	testCases := map[string]map[string]interface{}{
		"absent param": {},
		"empty string": {"command": ""},
		"blank string": {"command": "   "},
		"wrong type":   {"command": 123},
	}

	for name, params := range testCases {
		t.Run(name, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), params)

			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrAIToolParameterRequired)
			require.NotNil(t, result)
			assert.False(t, result.Success)
		})
	}
}

func TestCommandHelpTool_Execute_NoProviderConfigured(t *testing.T) {
	previous := commandTreeProvider
	SetCommandTreeProvider(nil)
	t.Cleanup(func() { SetCommandTreeProvider(previous) })

	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{"command": "vendor pull"})

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrAICommandTreeNotConfigured)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestCommandHelpTool_Execute_CommandNotFound(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})

	testCases := []string{"bogus", "vendor bogus", "vendor pull bogus"}
	for _, command := range testCases {
		t.Run(command, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]interface{}{"command": command})

			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrAICommandNotFound)
			require.NotNil(t, result)
			assert.False(t, result.Success)
		})
	}
}

func TestCommandHelpTool_Execute_ResolvesLeafWithCobraExample(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{"command": "vendor pull"})

	require.NoError(t, err)
	require.True(t, result.Success)

	assert.Equal(t, "vendor pull", result.Data["command"])
	assert.Equal(t, "Pull vendor dependencies", result.Data["short"])
	assert.Equal(t, "Pull and update vendor-specific configurations or dependencies.", result.Data["long"])
	assert.Equal(t, "  atmos vendor pull\n  atmos vendor pull --component vpc", result.Data["example"])
	assert.Equal(t, "cobra", result.Data["example_source"])

	flags, ok := result.Data["flags"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, flags, 2)
	assert.Equal(t, "component", flags[0]["name"])
	assert.Equal(t, "c", flags[0]["shorthand"])

	assert.Contains(t, result.Output, "atmos vendor pull")
	assert.Contains(t, result.Output, "Examples:")
	assert.Contains(t, result.Output, "Flags:")
	assert.Contains(t, result.Output, "component")
}

func TestCommandHelpTool_Execute_NoExampleAvailable(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewCommandHelpTool(&schema.AtmosConfiguration{})
	// "vendor diff" has no cobra Example and no commandUsageMarkdown entry.
	result, err := tool.Execute(context.Background(), map[string]interface{}{"command": "vendor diff"})

	require.NoError(t, err)
	require.True(t, result.Success)

	assert.Equal(t, "", result.Data["example"])
	assert.Equal(t, "", result.Data["example_source"])
	assert.Contains(t, result.Output, "Examples: none available for this command.")
	assert.Contains(t, result.Output, "Flags: none defined directly on this command.")
}

func TestResolveCommandExample(t *testing.T) {
	t.Run("prefers cobra example", func(t *testing.T) {
		node := &CommandNode{Example: "cobra example"}
		example, source := resolveCommandExample("about", node)
		assert.Equal(t, "cobra example", example)
		assert.Equal(t, "cobra", source)
	})

	t.Run("falls back to known markdown mapping", func(t *testing.T) {
		node := &CommandNode{}
		example, source := resolveCommandExample("about", node)
		assert.Equal(t, markdownContentFor(t, "about"), example)
		assert.Equal(t, "markdown", source)
	})

	t.Run("no example available for unmapped command", func(t *testing.T) {
		node := &CommandNode{}
		example, source := resolveCommandExample("vendor pull", node)
		assert.Equal(t, "", example)
		assert.Equal(t, "", source)
	})
}

// markdownContentFor is a small test helper to avoid hard-coding the
// embedded about.md content inline; it reads it back out of the same map
// the production code uses.
func markdownContentFor(t *testing.T, path string) string {
	t.Helper()
	content, ok := commandUsageMarkdown[path]
	require.True(t, ok)
	return content
}
