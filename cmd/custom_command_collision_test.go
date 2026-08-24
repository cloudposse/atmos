package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommand_NamespaceMerge_SubcommandsAdded tests that custom subcommands
// are merged into an existing built-in command's namespace when the top-level
// names collide.
func TestCustomCommand_NamespaceMerge_SubcommandsAdded(t *testing.T) {
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tk := NewTestKit(t)
	_ = tk

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Add a "builtin" command with a "sub1" subcommand to RootCmd,
	// simulating a built-in command registered via the command registry.
	builtinCmd := &cobra.Command{
		Use:   "builtin-ns",
		Short: "A built-in command",
	}
	builtinSub1 := &cobra.Command{
		Use:   "sub1",
		Short: "Built-in subcommand",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	builtinCmd.AddCommand(builtinSub1)
	RootCmd.AddCommand(builtinCmd)

	// Define a custom command with the same top-level name and a new subcommand.
	atmosConfig.Commands = []schema.Command{
		{
			Name:        "builtin-ns",
			Description: "Custom namespace container",
			Commands: []schema.Command{
				{
					Name:        "sub2",
					Description: "Custom subcommand added to built-in namespace",
					Steps:       stepsFromStrings("echo custom-sub2"),
				},
			},
		},
	}

	// Process custom commands.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Verify both subcommands exist under the built-in command.
	foundSub1 := false
	foundSub2 := false
	for _, c := range builtinCmd.Commands() {
		switch c.Name() {
		case "sub1":
			foundSub1 = true
		case "sub2":
			foundSub2 = true
		}
	}
	assert.True(t, foundSub1, "Built-in sub1 should still exist")
	assert.True(t, foundSub2, "Custom sub2 should be merged into built-in namespace")
}

// TestCustomCommand_NonTopLevelCollision_BuiltinPreserved tests that when a custom
// subcommand collides with an existing built-in subcommand at a non-top level,
// the built-in subcommand's behavior is preserved (not silently replaced).
func TestCustomCommand_NonTopLevelCollision_BuiltinPreserved(t *testing.T) {
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tk := NewTestKit(t)
	_ = tk

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a built-in command tree: "parent-cmd" > "start".
	parentCmd := &cobra.Command{
		Use:   "parent-cmd",
		Short: "A built-in parent command",
	}
	builtinStartRan := false
	builtinStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Built-in start subcommand",
		RunE: func(cmd *cobra.Command, args []string) error {
			builtinStartRan = true
			return nil
		},
	}
	parentCmd.AddCommand(builtinStartCmd)
	RootCmd.AddCommand(parentCmd)

	// Define custom commands that collide: "parent-cmd" with nested "start" (has steps)
	// and a new "custom-action" subcommand.
	atmosConfig.Commands = []schema.Command{
		{
			Name:        "parent-cmd",
			Description: "Custom override of parent",
			Commands: []schema.Command{
				{
					Name:        "start",
					Description: "Custom start that should NOT replace built-in",
					Steps:       stepsFromStrings("echo custom-start"),
				},
				{
					Name:        "custom-action",
					Description: "New custom subcommand alongside built-in",
					Steps:       stepsFromStrings("echo custom-action"),
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Find "start" under "parent-cmd".
	var startCmd *cobra.Command
	var customActionCmd *cobra.Command
	for _, c := range parentCmd.Commands() {
		switch c.Name() {
		case "start":
			startCmd = c
		case "custom-action":
			customActionCmd = c
		}
	}

	require.NotNil(t, startCmd, "start subcommand should exist")
	require.NotNil(t, customActionCmd, "custom-action subcommand should be added")

	// The critical assertion: the built-in start's RunE should still be intact,
	// not replaced by the custom command's steps handler.
	// Execute it and verify the built-in handler runs.
	if startCmd.RunE != nil {
		err = startCmd.RunE(startCmd, []string{})
		require.NoError(t, err)
	} else if startCmd.Run != nil {
		startCmd.Run(startCmd, []string{})
	}
	assert.True(t, builtinStartRan, "Built-in start's RunE should be preserved, not replaced by custom command")
}

// TestCustomCommand_DeepNesting_MergeWorks tests that custom commands can extend
// built-in command trees at arbitrary depth without replacing existing subcommands.
func TestCustomCommand_DeepNesting_MergeWorks(t *testing.T) {
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tk := NewTestKit(t)
	_ = tk

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create built-in: "deep" > "level1" (with RunE).
	deepCmd := &cobra.Command{
		Use:   "deep-cmd",
		Short: "Deep built-in",
	}
	level1Ran := false
	level1Cmd := &cobra.Command{
		Use:   "level1",
		Short: "Built-in level1",
		RunE: func(cmd *cobra.Command, args []string) error {
			level1Ran = true
			return nil
		},
	}
	deepCmd.AddCommand(level1Cmd)
	RootCmd.AddCommand(deepCmd)

	// Custom command: "deep-cmd" > "level1" > "level2" (container level1, leaf level2).
	atmosConfig.Commands = []schema.Command{
		{
			Name: "deep-cmd",
			Commands: []schema.Command{
				{
					Name:        "level1",
					Description: "Container to add level2 under built-in level1",
					Commands: []schema.Command{
						{
							Name:        "level2",
							Description: "New deep custom subcommand",
							Steps:       stepsFromStrings("echo level2"),
						},
					},
				},
			},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	// Verify level2 was added under level1.
	var level2Cmd *cobra.Command
	for _, c := range level1Cmd.Commands() {
		if c.Name() == "level2" {
			level2Cmd = c
		}
	}
	assert.NotNil(t, level2Cmd, "level2 should be added under built-in level1")

	// Verify level1's built-in RunE is preserved.
	if level1Cmd.RunE != nil {
		err = level1Cmd.RunE(level1Cmd, []string{})
		require.NoError(t, err)
	}
	assert.True(t, level1Ran, "Built-in level1 RunE should be preserved")
}
