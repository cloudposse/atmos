package atmos

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setTestCommandTree installs a fixed CommandNode tree for the duration of
// the test and restores the previous (process-global) provider afterward,
// since SetCommandTreeProvider mutates shared package state.
func setTestCommandTree(t *testing.T, roots []*CommandNode) {
	t.Helper()

	previous := commandTreeProvider
	SetCommandTreeProvider(func() []*CommandNode { return roots })
	t.Cleanup(func() { SetCommandTreeProvider(previous) })
}

// buildFixtureCommandTree returns a small, hand-built command tree used
// across list_commands and command_help tests. It includes a hidden
// top-level command and a hidden subcommand (to verify listing filters
// them out) plus a nested grandchild (to verify recursive vs non-recursive
// listing behaves differently).
func buildFixtureCommandTree() []*CommandNode {
	return []*CommandNode{
		{
			Name:  "vendor",
			Use:   "vendor",
			Short: "Manage vendored dependencies",
			Long:  "This command manages external dependencies for Atmos components or stacks by vendoring them.",
			Group: "Configuration Management",
			Subcommands: []*CommandNode{
				{
					Name:    "pull",
					Use:     "pull",
					Short:   "Pull vendor dependencies",
					Long:    "Pull and update vendor-specific configurations or dependencies.",
					Example: "  atmos vendor pull\n  atmos vendor pull --component vpc",
					Flags: []flagInfo{
						{Name: "component", Shorthand: "c", Description: "Component name", Default: ""},
						{Name: "everything", Description: "Vendor everything", Default: "false"},
					},
					Subcommands: []*CommandNode{
						{Name: "extra", Use: "extra", Short: "Nested grandchild for recursion tests"},
					},
				},
				{
					Name:  "diff",
					Use:   "diff",
					Short: "Show vendor diff",
				},
				{
					Name:   "hidden-sub",
					Use:    "hidden-sub",
					Short:  "should not appear",
					Hidden: true,
				},
			},
		},
		{
			Name:  "about",
			Use:   "about",
			Short: "Learn about Atmos",
			Group: "Other Commands",
		},
		{
			Name:   "hidden-top",
			Use:    "hidden-top",
			Short:  "should not appear",
			Hidden: true,
		},
	}
}

func TestNewListCommandsTool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	tool := NewListCommandsTool(atmosConfig)

	require.NotNil(t, tool)
	assert.Equal(t, atmosConfig, tool.atmosConfig)
}

func TestListCommandsTool_Name(t *testing.T) {
	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	assert.Equal(t, "atmos_list_commands", tool.Name())
}

func TestListCommandsTool_Description(t *testing.T) {
	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	assert.Contains(t, tool.Description(), "List Atmos CLI commands")
}

func TestListCommandsTool_Parameters(t *testing.T) {
	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	params := tool.Parameters()

	require.Len(t, params, 2)
	assert.Equal(t, "path", params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, "recursive", params[1].Name)
	assert.False(t, params[1].Required)
	assert.Equal(t, true, params[1].Default)
}

func TestListCommandsTool_RequiresPermission(t *testing.T) {
	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.RequiresPermission())
}

func TestListCommandsTool_IsRestricted(t *testing.T) {
	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	assert.False(t, tool.IsRestricted())
}

func TestListCommandsTool_Execute_NoProviderConfigured(t *testing.T) {
	previous := commandTreeProvider
	SetCommandTreeProvider(nil)
	t.Cleanup(func() { SetCommandTreeProvider(previous) })

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrAICommandTreeNotConfigured)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestListCommandsTool_Execute_TopLevelRecursive(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.True(t, result.Success)

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)

	var paths []string
	for _, c := range commands {
		paths = append(paths, c["path"].(string))
	}
	// Hidden top-level and hidden subcommand are excluded; full subtree included by default.
	assert.Equal(t, []string{"about", "vendor", "vendor diff", "vendor pull", "vendor pull extra"}, paths)
	assert.NotContains(t, paths, "hidden-top")
	assert.NotContains(t, paths, "vendor hidden-sub")

	assert.Contains(t, result.Output, "vendor pull")
	assert.Equal(t, true, result.Data["recursive"])
	assert.Equal(t, "", result.Data["path"])
}

func TestListCommandsTool_Execute_TopLevelNonRecursive(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{"recursive": false})

	require.NoError(t, err)
	require.True(t, result.Success)

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)

	var paths []string
	for _, c := range commands {
		paths = append(paths, c["path"].(string))
	}
	assert.Equal(t, []string{"about", "vendor"}, paths)
}

func TestListCommandsTool_Execute_ScopedPathRecursive(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": "vendor"})

	require.NoError(t, err)
	require.True(t, result.Success)

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)

	var paths []string
	for _, c := range commands {
		paths = append(paths, c["path"].(string))
	}
	assert.Equal(t, []string{"vendor diff", "vendor pull", "vendor pull extra"}, paths)
	assert.Contains(t, result.Output, `under "vendor"`)
}

func TestListCommandsTool_Execute_ScopedPathNonRecursive(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":      "vendor",
		"recursive": false,
	})

	require.NoError(t, err)
	require.True(t, result.Success)

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)

	var paths []string
	for _, c := range commands {
		paths = append(paths, c["path"].(string))
	}
	// Non-recursive: "vendor pull extra" (the grandchild) must NOT appear.
	assert.Equal(t, []string{"vendor diff", "vendor pull"}, paths)
}

func TestListCommandsTool_Execute_PathNotFound(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})

	testCases := []string{"bogus", "vendor bogus"}
	for _, path := range testCases {
		t.Run(path, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), map[string]interface{}{"path": path})

			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrAICommandNotFound)
			require.NotNil(t, result)
			assert.False(t, result.Success)
		})
	}
}

func TestListCommandsTool_Execute_LeafPathHasNoChildren(t *testing.T) {
	setTestCommandTree(t, buildFixtureCommandTree())

	tool := NewListCommandsTool(&schema.AtmosConfiguration{})
	result, err := tool.Execute(context.Background(), map[string]interface{}{"path": "vendor diff"})

	require.NoError(t, err)
	require.True(t, result.Success)

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, commands)
}

func TestFindNodeByPath(t *testing.T) {
	roots := buildFixtureCommandTree()

	t.Run("resolves top-level", func(t *testing.T) {
		node, err := findNodeByPath(roots, "about")
		require.NoError(t, err)
		require.NotNil(t, node)
		assert.Equal(t, "about", node.Name)
	})

	t.Run("resolves nested", func(t *testing.T) {
		node, err := findNodeByPath(roots, "vendor pull extra")
		require.NoError(t, err)
		require.NotNil(t, node)
		assert.Equal(t, "extra", node.Name)
	})

	t.Run("resolves hidden node directly", func(t *testing.T) {
		// Hidden nodes are excluded from *listings* but still directly resolvable,
		// matching typical CLI "hidden but runnable" semantics.
		node, err := findNodeByPath(roots, "vendor hidden-sub")
		require.NoError(t, err)
		require.NotNil(t, node)
	})

	t.Run("empty path", func(t *testing.T) {
		node, err := findNodeByPath(roots, "")
		require.Error(t, err)
		require.ErrorIs(t, err, errUtils.ErrAICommandNotFound)
		assert.Nil(t, node)
	})

	t.Run("unknown top-level", func(t *testing.T) {
		node, err := findNodeByPath(roots, "bogus")
		require.Error(t, err)
		require.ErrorIs(t, err, errUtils.ErrAICommandNotFound)
		assert.Nil(t, node)
	})

	t.Run("unknown nested", func(t *testing.T) {
		node, err := findNodeByPath(roots, "vendor bogus")
		require.Error(t, err)
		require.ErrorIs(t, err, errUtils.ErrAICommandNotFound)
		assert.Nil(t, node)
	})
}

func TestSortedNodes(t *testing.T) {
	nodes := []*CommandNode{
		{Name: "zeta"},
		nil,
		{Name: "alpha"},
		{Name: "hidden", Hidden: true},
	}

	visible := sortedNodes(nodes)

	require.Len(t, visible, 2)
	assert.Equal(t, "alpha", visible[0].Name)
	assert.Equal(t, "zeta", visible[1].Name)
}

func TestNewCommandNodeFromCobra(t *testing.T) {
	child := &cobra.Command{
		Use:     "pull [component]",
		Short:   "pull short",
		Long:    "pull long",
		Example: "example text",
	}
	child.Flags().StringP("component", "c", "", "component flag desc")
	child.Flags().Bool("everything", false, "vendor everything")
	child.Flags().Bool("secret", false, "hidden flag")
	require.NoError(t, child.Flags().MarkHidden("secret"))

	hiddenChild := &cobra.Command{Use: "hidden", Short: "hidden short", Hidden: true}

	parent := &cobra.Command{Use: "vendor", Short: "vendor short", Long: "vendor long"}
	parent.AddCommand(child, hiddenChild)

	node := NewCommandNodeFromCobra(parent, "Configuration Management")

	assert.Equal(t, "vendor", node.Name)
	assert.Equal(t, "vendor short", node.Short)
	assert.Equal(t, "vendor long", node.Long)
	assert.Equal(t, "Configuration Management", node.Group)
	assert.False(t, node.Hidden)
	require.Len(t, node.Subcommands, 2)

	var pullNode, hiddenNode *CommandNode
	for _, c := range node.Subcommands {
		switch c.Name {
		case "pull":
			pullNode = c
		case "hidden":
			hiddenNode = c
		}
	}

	require.NotNil(t, pullNode)
	require.NotNil(t, hiddenNode)

	assert.True(t, hiddenNode.Hidden)
	assert.Equal(t, "", hiddenNode.Group, "descendants must not inherit the parent's group")

	assert.False(t, pullNode.Hidden)
	assert.Equal(t, "pull [component]", pullNode.Use)
	assert.Equal(t, "pull short", pullNode.Short)
	assert.Equal(t, "pull long", pullNode.Long)
	assert.Equal(t, "example text", pullNode.Example)
	assert.Equal(t, "", pullNode.Group)
	assert.Empty(t, pullNode.Subcommands)

	require.Len(t, pullNode.Flags, 2, "the hidden 'secret' flag must be filtered out")
	assert.Equal(t, "component", pullNode.Flags[0].Name)
	assert.Equal(t, "c", pullNode.Flags[0].Shorthand)
	assert.Equal(t, "component flag desc", pullNode.Flags[0].Description)
	assert.Equal(t, "everything", pullNode.Flags[1].Name)
	assert.Equal(t, "false", pullNode.Flags[1].Default)

	for _, f := range pullNode.Flags {
		assert.NotEqual(t, "secret", f.Name)
	}
}

func TestBuildListCommandsResult(t *testing.T) {
	entries := []commandEntry{
		{Path: "about", Use: "about", Short: "Learn about Atmos", Group: "Other Commands"},
		{Path: "vendor", Use: "vendor", Short: "", Group: "Configuration Management"},
	}

	result := buildListCommandsResult(entries, "", true)

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Available Atmos Commands (2):")
	assert.Contains(t, result.Output, "about")
	assert.Contains(t, result.Output, "vendor")

	commands, ok := result.Data["commands"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, commands, 2)
	assert.Equal(t, "about", commands[0]["path"])
	assert.Equal(t, "Learn about Atmos", commands[0]["short"])
	assert.Equal(t, "Other Commands", commands[0]["group"])
}
