package cmd

import (
	"strings"
	"testing"

	cockerrors "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommand_FlagNameConflictWithGlobalFlag tests that custom commands with flags
// that conflict with global flags return a proper error instead of panicking.
func TestCustomCommand_FlagNameConflictWithGlobalFlag(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with a flag that has the same name as global --mask
	// but DIFFERENT type (string instead of bool). This should error.
	testCommand := schema.Command{
		Name:        "test-flag-conflict",
		Description: "Test command with type-conflicting flag name",
		Flags: []schema.CommandFlag{
			{
				Name:  "mask",
				Type:  "string", // Global mask is bool - type mismatch!
				Usage: "This conflicts with global --mask flag (different type)",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error for type mismatch.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for type mismatch")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "mask", "Error details should mention the conflicting flag name")
	assert.Contains(t, detailsStr, "test-flag-conflict", "Error details should mention the command name")
	assert.Contains(t, detailsStr, "type", "Error details should mention type mismatch")
}

// TestCustomCommand_FlagShorthandConflictWithGlobalFlag tests that custom commands with flag
// shorthands that conflict with global flag shorthands return a proper error.
func TestCustomCommand_FlagShorthandConflictWithGlobalFlag(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with a flag shorthand that conflicts with --chdir (-C).
	testCommand := schema.Command{
		Name:        "test-shorthand-conflict",
		Description: "Test command with conflicting flag shorthand",
		Flags: []schema.CommandFlag{
			{
				Name:      "custom-flag",
				Shorthand: "C", // Conflicts with global --chdir shorthand.
				Type:      "string",
				Usage:     "This has a conflicting shorthand",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error, not panic.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for conflicting flag shorthand")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "-C", "Error details should mention the conflicting shorthand")
	assert.Contains(t, detailsStr, "test-shorthand-conflict", "Error details should mention the command name")
}

// TestCustomCommand_IdentityFlagConflict tests that custom commands can declare
// they need --identity flag (with matching type), but cannot redefine it with different type.
func TestCustomCommand_IdentityFlagConflict(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command that tries to redefine identity flag with DIFFERENT type.
	// The identity flag added to custom commands is always string type.
	testCommand := schema.Command{
		Name:        "test-identity-conflict",
		Description: "Test command trying to redefine identity flag with wrong type",
		Flags: []schema.CommandFlag{
			{
				Name:  "identity",
				Type:  "bool", // Wrong type! Identity is string.
				Usage: "This conflicts with reserved identity flag (type mismatch)",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error for type mismatch.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for type mismatch on identity flag")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "identity", "Error details should mention the identity flag")
	assert.Contains(t, detailsStr, "type", "Error details should mention type mismatch")
}

// TestCustomCommand_ValidFlagsNoConflict tests that custom commands with valid,
// non-conflicting flags are registered successfully.
func TestCustomCommand_ValidFlagsNoConflict(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with valid, non-conflicting flags.
	testCommand := schema.Command{
		Name:        "test-valid-flags",
		Description: "Test command with valid flags",
		Flags: []schema.CommandFlag{
			{
				Name:      "my-custom-flag",
				Shorthand: "m",
				Type:      "string",
				Usage:     "A valid custom flag",
				Default:   "default-value",
			},
			{
				Name:  "another-flag",
				Type:  "bool",
				Usage: "Another valid flag",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with valid, non-conflicting flags")

	// Find and verify the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-valid-flags" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify flags are registered correctly.
	myFlag := customCmd.PersistentFlags().Lookup("my-custom-flag")
	require.NotNil(t, myFlag, "my-custom-flag should be registered")
	assert.Equal(t, "m", myFlag.Shorthand, "Shorthand should be 'm'")

	anotherFlag := customCmd.PersistentFlags().Lookup("another-flag")
	require.NotNil(t, anotherFlag, "another-flag should be registered")
}

// TestGetGlobalFlagNames tests that getGlobalFlagNames returns the expected reserved flags.
// TestGetGlobalFlagNames was removed.
// The implementation no longer uses a pre-computed set of reserved flags.
// Instead, flags are validated dynamically by checking if they already exist
// on parent commands, allowing inheritance when types match.

// TestCustomCommand_NestedFlagConflictWithParent tests that nested custom commands
// cannot redefine flags with different types than their parent's flags.
// Child can declare same flag with same type (inheritance), but different type is an error.
func TestCustomCommand_NestedFlagConflictWithParent(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a parent custom command with a persistent flag.
	parentCommand := schema.Command{
		Name:        "test-parent",
		Description: "Parent command with a persistent flag",
		Flags: []schema.CommandFlag{
			{
				Name:      "parent-flag",
				Shorthand: "p",
				Type:      "string",
				Usage:     "A flag defined on the parent command",
			},
		},
		// Define a nested child command that tries to redefine the same flag with DIFFERENT type.
		Commands: []schema.Command{
			{
				Name:        "child",
				Description: "Child command trying to redefine parent's flag with different type",
				Flags: []schema.CommandFlag{
					{
						Name:  "parent-flag", // Same name as parent's flag.
						Type:  "bool",        // But different type (parent is string)!
						Usage: "This conflicts with parent's flag (type mismatch)",
					},
				},
				Steps: stepsFromStrings("echo child"),
			},
		},
		Steps: stepsFromStrings("echo parent"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{parentCommand}

	// Process custom commands - should return error for type mismatch.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for type mismatch")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "parent-flag", "Error details should mention the conflicting flag name")
	assert.Contains(t, detailsStr, "child", "Error details should mention the child command name")
	assert.Contains(t, detailsStr, "type", "Error details should mention type mismatch")
}

// TestCustomCommand_NestedShorthandConflictWithParent tests that nested custom commands
// cannot define flag shorthands that conflict with their parent's flag shorthands.
func TestCustomCommand_NestedShorthandConflictWithParent(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a parent custom command with a persistent flag that has a shorthand.
	parentCommand := schema.Command{
		Name:        "test-parent-shorthand",
		Description: "Parent command with a flag shorthand",
		Flags: []schema.CommandFlag{
			{
				Name:      "parent-option",
				Shorthand: "x",
				Type:      "string",
				Usage:     "A flag with shorthand -x on the parent",
			},
		},
		// Define a nested child command that tries to use the same shorthand.
		Commands: []schema.Command{
			{
				Name:        "child",
				Description: "Child command trying to reuse parent's shorthand",
				Flags: []schema.CommandFlag{
					{
						Name:      "child-option",
						Shorthand: "x", // Conflicts with parent's -x shorthand.
						Type:      "bool",
						Usage:     "This has a conflicting shorthand",
					},
				},
				Steps: stepsFromStrings("echo child"),
			},
		},
		Steps: stepsFromStrings("echo parent"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{parentCommand}

	// Process custom commands - should return error for the shorthand conflict.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for nested shorthand conflict with parent")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "-x", "Error details should mention the conflicting shorthand")
	assert.Contains(t, detailsStr, "child", "Error details should mention the child command name")
}

// TestCustomCommand_NestedValidFlags tests that nested custom commands with valid,
// non-conflicting flags are registered successfully.
func TestCustomCommand_NestedValidFlags(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a parent custom command with a persistent flag.
	parentCommand := schema.Command{
		Name:        "test-nested-valid",
		Description: "Parent command with a persistent flag",
		Flags: []schema.CommandFlag{
			{
				Name:      "parent-flag",
				Shorthand: "p",
				Type:      "string",
				Usage:     "A flag defined on the parent command",
			},
		},
		// Define a nested child command with different, non-conflicting flags.
		Commands: []schema.Command{
			{
				Name:        "child",
				Description: "Child command with its own unique flag",
				Flags: []schema.CommandFlag{
					{
						Name:      "child-flag",
						Shorthand: "c",
						Type:      "bool",
						Usage:     "A unique flag for the child command",
					},
				},
				Steps: stepsFromStrings("echo child"),
			},
		},
		Steps: stepsFromStrings("echo parent"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{parentCommand}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with valid, non-conflicting nested flags")

	// Find and verify the parent command.
	var parentCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-nested-valid" {
			parentCmd = cmd
			break
		}
	}
	require.NotNil(t, parentCmd, "Parent command should be registered")

	// Verify parent flag is registered.
	parentFlag := parentCmd.PersistentFlags().Lookup("parent-flag")
	require.NotNil(t, parentFlag, "parent-flag should be registered on parent")

	// Find and verify the child command.
	var childCmd *cobra.Command
	for _, cmd := range parentCmd.Commands() {
		if cmd.Name() == "child" {
			childCmd = cmd
			break
		}
	}
	require.NotNil(t, childCmd, "Child command should be registered")

	// Verify child flag is registered.
	childFlag := childCmd.PersistentFlags().Lookup("child-flag")
	require.NotNil(t, childFlag, "child-flag should be registered on child")

	// Verify that child can still access parent's flag via inherited flags.
	inheritedFlag := childCmd.InheritedFlags().Lookup("parent-flag")
	require.NotNil(t, inheritedFlag, "parent-flag should be inherited by child")
}

// TestGetReservedFlagNamesFor was removed.
// The implementation no longer uses a pre-computed set of reserved flags.
// Instead, flags are validated dynamically by checking if they already exist
// on parent commands, allowing inheritance when types match.

// TestCustomCommand_ExistingCommandReuseWithNestedConflict tests that when a custom command
// reuses an existing built-in command (like terraform), nested subcommands can declare they
// need the same flags (inheritance), but cannot redefine them with different types.
func TestCustomCommand_ExistingCommandReuseWithNestedConflict(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Preconditions: RootCmd must have the built-in terraform command and its stack flag.
	// This prevents false negatives if the terraform command is ever renamed or removed.
	tfCmd, _, tfErr := RootCmd.Find([]string{"terraform"})
	require.NoError(t, tfErr, "Precondition: terraform command must exist")
	require.NotNil(t, tfCmd, "Precondition: terraform command must be registered")
	// Check PersistentFlags() since the stack flag is defined directly on terraform, not inherited.
	require.NotNil(t, tfCmd.PersistentFlags().Lookup("stack"), "Precondition: terraform must have --stack flag")

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command that reuses "terraform" (built-in command name)
	// with a nested subcommand that tries to redefine --stack with DIFFERENT type.
	terraformCommand := schema.Command{
		Name:        "terraform", // This will reuse the existing terraform command.
		Description: "Custom terraform subcommands",
		Commands: []schema.Command{
			{
				Name:        "custom-provision",
				Description: "Custom provision subcommand",
				Flags: []schema.CommandFlag{
					{
						Name:      "stack", // Same name as terraform's --stack flag.
						Shorthand: "s",     // Same shorthand.
						Type:      "bool",  // But different type! Terraform's stack is string.
						Usage:     "This conflicts with terraform's stack flag (type mismatch)",
					},
				},
				Steps: stepsFromStrings("echo provision"),
			},
		},
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{terraformCommand}

	// Process custom commands - should return error for the nested conflict.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for nested flag conflict with built-in terraform")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "stack", "Error details should mention the conflicting flag name")
	assert.Contains(t, detailsStr, "custom-provision", "Error details should mention the custom subcommand name")
}

// TestCustomCommand_BoolFlagWithStringDefault tests that bool flags handle
// non-bool default values gracefully (they should be treated as false).
func TestCustomCommand_BoolFlagWithStringDefault(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with a bool flag that has a non-bool default.
	testCommand := schema.Command{
		Name:        "test-bool-string-default",
		Description: "Test bool flag with string default",
		Flags: []schema.CommandFlag{
			{
				Name:    "my-bool",
				Type:    "bool",
				Usage:   "A bool flag with string default",
				Default: "not-a-bool", // This should be treated as false.
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should succeed (default falls back to false).
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed even with non-bool default for bool flag")

	// Find and verify the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-bool-string-default" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify the flag has default value of false.
	myBool := customCmd.PersistentFlags().Lookup("my-bool")
	require.NotNil(t, myBool, "my-bool should be registered")
	assert.Equal(t, "false", myBool.DefValue, "Default should be false for non-bool default")
}

// TestCustomCommand_FlagDefaultTypeConversions tests that flags handle various default value types.
// This is a table-driven test that covers: string flags with bool defaults, bool flags with true defaults.
func TestCustomCommand_FlagDefaultTypeConversions(t *testing.T) {
	tests := []struct {
		name            string
		commandName     string
		flagName        string
		flagType        string
		flagDefault     any
		expectedDefault string
	}{
		{
			name:            "string_flag_with_bool_default",
			commandName:     "test-string-bool-default",
			flagName:        "my-string",
			flagType:        "string",
			flagDefault:     true, // Non-string default should be treated as empty string.
			expectedDefault: "",
		},
		{
			name:            "bool_flag_with_true_default",
			commandName:     "test-bool-true-default",
			flagName:        "enabled",
			flagType:        "bool",
			flagDefault:     true,
			expectedDefault: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test fixture.
			testDir := "../tests/fixtures/scenarios/complete"
			t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
			t.Setenv("ATMOS_BASE_PATH", testDir)

			// Create a test kit to ensure clean RootCmd state.
			_ = NewTestKit(t)

			// Load atmos configuration.
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			require.NoError(t, err)

			// Create a custom command with the specified flag configuration.
			testCommand := schema.Command{
				Name:        tt.commandName,
				Description: "Test command for " + tt.name,
				Flags: []schema.CommandFlag{
					{
						Name:    tt.flagName,
						Type:    tt.flagType,
						Usage:   "Test flag",
						Default: tt.flagDefault,
					},
				},
				Steps: stepsFromStrings("echo test"),
			}

			// Add the test command to the config.
			atmosConfig.Commands = []schema.Command{testCommand}

			// Process custom commands - should succeed.
			err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
			require.NoError(t, err, "Should succeed processing command")

			// Find and verify the custom command.
			var customCmd *cobra.Command
			for _, cmd := range RootCmd.Commands() {
				if cmd.Name() == tt.commandName {
					customCmd = cmd
					break
				}
			}
			require.NotNil(t, customCmd, "Custom command should be registered")

			// Verify the flag has the expected default value.
			flag := customCmd.PersistentFlags().Lookup(tt.flagName)
			require.NotNil(t, flag, "Flag should be registered")
			assert.Equal(t, tt.expectedDefault, flag.DefValue, "Default value mismatch")
		})
	}
}

// TestCustomCommand_EmptyFlagsList tests that commands with no flags are processed correctly.
func TestCustomCommand_EmptyFlagsList(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with no flags.
	testCommand := schema.Command{
		Name:        "test-no-flags",
		Description: "Test command with no flags",
		Flags:       []schema.CommandFlag{}, // Empty flags list.
		Steps:       stepsFromStrings("echo no flags"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with empty flags list")

	// Find and verify the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-no-flags" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify it still has the identity flag (added automatically).
	identityFlag := customCmd.PersistentFlags().Lookup("identity")
	require.NotNil(t, identityFlag, "identity flag should still be added automatically")
}

// TestCustomCommand_ContainerCommandWithSubcommands tests commands that have
// no steps but have subcommands (container commands).
func TestCustomCommand_ContainerCommandWithSubcommands(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a container command (no steps, only subcommands).
	containerCommand := schema.Command{
		Name:        "test-container",
		Description: "Container command with no steps",
		// No Steps field - this is a container command.
		Commands: []schema.Command{
			{
				Name:        "sub1",
				Description: "First subcommand",
				Steps:       stepsFromStrings("echo sub1"),
			},
			{
				Name:        "sub2",
				Description: "Second subcommand",
				Steps:       stepsFromStrings("echo sub2"),
			},
		},
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{containerCommand}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with container command")

	// Find and verify the container command.
	var containerCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-container" {
			containerCmd = cmd
			break
		}
	}
	require.NotNil(t, containerCmd, "Container command should be registered")

	// Verify subcommands are registered.
	subCmds := containerCmd.Commands()
	assert.Len(t, subCmds, 2, "Container should have 2 subcommands")

	subCmdNames := make([]string, len(subCmds))
	for i, cmd := range subCmds {
		subCmdNames[i] = cmd.Name()
	}
	assert.Contains(t, subCmdNames, "sub1", "sub1 should be registered")
	assert.Contains(t, subCmdNames, "sub2", "sub2 should be registered")
}

// TestCustomCommand_DeeplyNestedCommands tests commands nested 3+ levels deep.
func TestCustomCommand_DeeplyNestedCommands(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a deeply nested command structure (3 levels).
	level1Command := schema.Command{
		Name:        "level1",
		Description: "Level 1 command",
		Flags: []schema.CommandFlag{
			{Name: "l1-flag", Type: "string", Usage: "Level 1 flag"},
		},
		Commands: []schema.Command{
			{
				Name:        "level2",
				Description: "Level 2 command",
				Flags: []schema.CommandFlag{
					{Name: "l2-flag", Type: "string", Usage: "Level 2 flag"},
				},
				Commands: []schema.Command{
					{
						Name:        "level3",
						Description: "Level 3 command",
						Flags: []schema.CommandFlag{
							{Name: "l3-flag", Type: "string", Usage: "Level 3 flag"},
						},
						Steps: stepsFromStrings("echo level3"),
					},
				},
			},
		},
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{level1Command}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with deeply nested commands")

	// Find and verify level1.
	var l1Cmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "level1" {
			l1Cmd = cmd
			break
		}
	}
	require.NotNil(t, l1Cmd, "Level 1 command should be registered")
	assert.NotNil(t, l1Cmd.PersistentFlags().Lookup("l1-flag"), "l1-flag should exist")

	// Find level2.
	var l2Cmd *cobra.Command
	for _, cmd := range l1Cmd.Commands() {
		if cmd.Name() == "level2" {
			l2Cmd = cmd
			break
		}
	}
	require.NotNil(t, l2Cmd, "Level 2 command should be registered")
	assert.NotNil(t, l2Cmd.PersistentFlags().Lookup("l2-flag"), "l2-flag should exist")

	// Verify level2 inherits level1's flag.
	assert.NotNil(t, l2Cmd.InheritedFlags().Lookup("l1-flag"), "l2 should inherit l1-flag")

	// Find level3.
	var l3Cmd *cobra.Command
	for _, cmd := range l2Cmd.Commands() {
		if cmd.Name() == "level3" {
			l3Cmd = cmd
			break
		}
	}
	require.NotNil(t, l3Cmd, "Level 3 command should be registered")
	assert.NotNil(t, l3Cmd.PersistentFlags().Lookup("l3-flag"), "l3-flag should exist")

	// Verify level3 inherits both level1 and level2 flags.
	assert.NotNil(t, l3Cmd.InheritedFlags().Lookup("l1-flag"), "l3 should inherit l1-flag")
	assert.NotNil(t, l3Cmd.InheritedFlags().Lookup("l2-flag"), "l3 should inherit l2-flag")
}

// TestCustomCommand_DeeplyNestedConflict tests that flag conflicts are detected
// at any level of nesting (e.g., level 3 conflict with level 1 flag).
func TestCustomCommand_DeeplyNestedConflict(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a deeply nested command structure where level3 conflicts with level1.
	level1Command := schema.Command{
		Name:        "deep-conflict-test",
		Description: "Level 1 command",
		Flags: []schema.CommandFlag{
			{Name: "ancestor-flag", Type: "string", Usage: "Flag on ancestor"},
		},
		Commands: []schema.Command{
			{
				Name:        "level2",
				Description: "Level 2 command",
				Commands: []schema.Command{
					{
						Name:        "level3",
						Description: "Level 3 command that conflicts with level 1",
						Flags: []schema.CommandFlag{
							{Name: "ancestor-flag", Type: "bool", Usage: "Conflicts with level1"}, // Conflict!
						},
						Steps: stepsFromStrings("echo level3"),
					},
				},
			},
		},
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{level1Command}

	// Process custom commands - should return error.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for deep nested flag conflict")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "ancestor-flag", "Error details should mention the conflicting flag")
	assert.Contains(t, detailsStr, "level3", "Error details should mention the command name")
}

// TestCustomCommand_MultipleCommandsWithMixedValidity tests processing multiple
// commands where some are valid and some have conflicts.
//
// Note: This test documents the current "fail-fast" behavior where valid commands registered
// before the first error remain registered. This is intentional - configuration errors should
// be fixed rather than silently ignored. If "all-or-nothing" semantics are desired in the
// future, a pre-validation pass would be needed before registration.
func TestCustomCommand_MultipleCommandsWithMixedValidity(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create multiple commands - first valid, second has type conflict.
	commands := []schema.Command{
		{
			Name:        "valid-cmd-1",
			Description: "First valid command",
			Flags: []schema.CommandFlag{
				{Name: "valid-flag-1", Type: "string", Usage: "Valid flag"},
			},
			Steps: stepsFromStrings("echo valid1"),
		},
		{
			Name:        "invalid-cmd",
			Description: "Command with type mismatch",
			Flags: []schema.CommandFlag{
				{Name: "mask", Type: "string", Usage: "Type mismatch - global mask is bool"}, // Type conflict!
			},
			Steps: stepsFromStrings("echo invalid"),
		},
		{
			Name:        "valid-cmd-2",
			Description: "Second valid command (won't be processed due to earlier error)",
			Flags: []schema.CommandFlag{
				{Name: "valid-flag-2", Type: "string", Usage: "Valid flag"},
			},
			Steps: stepsFromStrings("echo valid2"),
		},
	}

	// Add the test commands to the config.
	atmosConfig.Commands = commands

	// Process custom commands - should return error for the invalid command.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName when any command has conflict")

	// Verify that valid-cmd-1 was registered before the error occurred.
	var validCmd1 *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "valid-cmd-1" {
			validCmd1 = cmd
			break
		}
	}
	require.NotNil(t, validCmd1, "First valid command should be registered before error")

	// Verify that valid-cmd-2 was NOT registered (processing stopped at error).
	var validCmd2 *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "valid-cmd-2" {
			validCmd2 = cmd
			break
		}
	}
	assert.Nil(t, validCmd2, "Second valid command should NOT be registered after error")
}

// TestCustomCommand_RequiredFlagMarking tests that required flags are marked correctly.
func TestCustomCommand_RequiredFlagMarking(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with required and optional flags.
	testCommand := schema.Command{
		Name:        "test-required-flags",
		Description: "Test command with required and optional flags",
		Flags: []schema.CommandFlag{
			{
				Name:     "required-flag",
				Type:     "string",
				Usage:    "This flag is required",
				Required: true,
			},
			{
				Name:     "optional-flag",
				Type:     "string",
				Usage:    "This flag is optional",
				Required: false,
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should succeed.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err, "Should succeed with required flags")

	// Find and verify the custom command.
	var customCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-required-flags" {
			customCmd = cmd
			break
		}
	}
	require.NotNil(t, customCmd, "Custom command should be registered")

	// Verify required flag annotation.
	requiredFlag := customCmd.PersistentFlags().Lookup("required-flag")
	require.NotNil(t, requiredFlag, "required-flag should be registered")
	// Check if the flag has required annotation.
	annotations := requiredFlag.Annotations
	_, hasRequired := annotations[cobra.BashCompOneRequiredFlag]
	assert.True(t, hasRequired, "required-flag should have required annotation")

	// Verify optional flag has no required annotation.
	optionalFlag := customCmd.PersistentFlags().Lookup("optional-flag")
	require.NotNil(t, optionalFlag, "optional-flag should be registered")
	if optionalFlag.Annotations != nil {
		_, hasRequired = optionalFlag.Annotations[cobra.BashCompOneRequiredFlag]
		assert.False(t, hasRequired, "optional-flag should NOT have required annotation")
	}
}

// TestCustomCommand_VerboseFlagConflict tests that the verbose flag (-v) is correctly
// detected as a conflict since it's a global flag.
func TestCustomCommand_VerboseFlagConflict(t *testing.T) {
	// Set up test fixture.
	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Create a custom command with verbose flag but DIFFERENT type (type mismatch).
	testCommand := schema.Command{
		Name:        "test-verbose-conflict",
		Description: "Test command with verbose flag type mismatch",
		Flags: []schema.CommandFlag{
			{
				Name:      "verbose",
				Shorthand: "v",
				Type:      "string", // Global verbose is bool - type mismatch!
				Usage:     "This conflicts with global verbose flag (different type)",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error for type mismatch.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.ErrorIs(t, err, errUtils.ErrReservedFlagName, "Should return ErrReservedFlagName for type mismatch")
}

// customCommandTestHelper provides shared setup for custom command tests.
type customCommandTestHelper struct {
	atmosConfig schema.AtmosConfiguration
}

// newCustomCommandTestHelper creates a new test helper with common setup.
func newCustomCommandTestHelper(t *testing.T) *customCommandTestHelper {
	t.Helper()

	testDir := "../tests/fixtures/scenarios/complete"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	return &customCommandTestHelper{
		atmosConfig: atmosConfig,
	}
}

// processCommand processes a single command and returns the error.
func (h *customCommandTestHelper) processCommand(cmd *schema.Command) error {
	h.atmosConfig.Commands = []schema.Command{*cmd}
	return processCustomCommands(h.atmosConfig, h.atmosConfig.Commands, RootCmd, true)
}

// TestCustomCommand_DuplicateFlagName tests that duplicate flag names within the same
// command are detected and return an error.
func TestCustomCommand_DuplicateFlagName(t *testing.T) {
	helper := newCustomCommandTestHelper(t)

	testCommand := &schema.Command{
		Name:        "test-duplicate-flag",
		Description: "Test command with duplicate flag names",
		Flags: []schema.CommandFlag{
			{
				Name:  "my-flag",
				Type:  "string",
				Usage: "First definition of my-flag",
			},
			{
				Name:  "my-flag", // Duplicate!
				Type:  "bool",
				Usage: "Second definition of my-flag",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	err := helper.processCommand(testCommand)

	require.ErrorIs(t, err, errUtils.ErrDuplicateFlagRegistration, "Should return ErrDuplicateFlagRegistration for duplicate flag name")

	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "my-flag", "Error details should mention the duplicate flag")
	assert.Contains(t, detailsStr, "test-duplicate-flag", "Error details should mention the command name")
}

// TestCustomCommand_DuplicateShorthand tests that duplicate flag shorthands within
// the same command are detected and return an error.
func TestCustomCommand_DuplicateShorthand(t *testing.T) {
	helper := newCustomCommandTestHelper(t)

	testCommand := &schema.Command{
		Name:        "test-duplicate-shorthand",
		Description: "Test command with duplicate flag shorthands",
		Flags: []schema.CommandFlag{
			{
				Name:      "first-flag",
				Shorthand: "f",
				Type:      "string",
				Usage:     "First flag with -f shorthand",
			},
			{
				Name:      "second-flag",
				Shorthand: "f", // Duplicate shorthand!
				Type:      "bool",
				Usage:     "Second flag also trying to use -f",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	err := helper.processCommand(testCommand)

	require.ErrorIs(t, err, errUtils.ErrDuplicateFlagRegistration, "Should return ErrDuplicateFlagRegistration for duplicate shorthand")

	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "-f", "Error details should mention the duplicate shorthand")
	assert.Contains(t, detailsStr, "test-duplicate-shorthand", "Error details should mention the command name")
}

// TestCustomCommand_ShorthandMatchesFlagName tests that a shorthand matching another
// flag's name is detected as a duplicate.
func TestCustomCommand_ShorthandMatchesFlagName(t *testing.T) {
	helper := newCustomCommandTestHelper(t)

	testCommand := &schema.Command{
		Name:        "test-shorthand-matches-name",
		Description: "Test shorthand that matches another flag's name",
		Flags: []schema.CommandFlag{
			{
				Name:  "x",
				Type:  "string",
				Usage: "A flag named 'x'",
			},
			{
				Name:      "extended",
				Shorthand: "x", // Shorthand matches the name of the first flag!
				Type:      "bool",
				Usage:     "Flag with -x shorthand",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	err := helper.processCommand(testCommand)

	require.ErrorIs(t, err, errUtils.ErrDuplicateFlagRegistration, "Should return ErrDuplicateFlagRegistration when shorthand matches another flag's name")
}
