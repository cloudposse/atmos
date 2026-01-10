package cmd

import (
	"errors"
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

	// Create a custom command with a flag that conflicts with the global --mask flag.
	testCommand := schema.Command{
		Name:        "test-flag-conflict",
		Description: "Test command with conflicting flag name",
		Flags: []schema.CommandFlag{
			{
				Name:  "mask",
				Type:  "bool",
				Usage: "This conflicts with global --mask flag",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error, not panic.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.Error(t, err, "Should return error for conflicting flag name")
	assert.True(t, errors.Is(err, errUtils.ErrReservedFlagName), "Error should be ErrReservedFlagName")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "mask", "Error details should mention the conflicting flag name")
	assert.Contains(t, detailsStr, "test-flag-conflict", "Error details should mention the command name")
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
	require.Error(t, err, "Should return error for conflicting flag shorthand")
	assert.True(t, errors.Is(err, errUtils.ErrReservedFlagName), "Error should be ErrReservedFlagName")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "-C", "Error details should mention the conflicting shorthand")
	assert.Contains(t, detailsStr, "test-shorthand-conflict", "Error details should mention the command name")
}

// TestCustomCommand_IdentityFlagConflict tests that custom commands cannot define
// an --identity flag since it's reserved for runtime identity override.
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

	// Create a custom command that tries to define its own --identity flag.
	testCommand := schema.Command{
		Name:        "test-identity-conflict",
		Description: "Test command trying to redefine identity flag",
		Flags: []schema.CommandFlag{
			{
				Name:  "identity",
				Type:  "string",
				Usage: "This conflicts with reserved identity flag",
			},
		},
		Steps: stepsFromStrings("echo test"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{testCommand}

	// Process custom commands - should return error, not panic.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.Error(t, err, "Should return error for conflicting identity flag")
	assert.True(t, errors.Is(err, errUtils.ErrReservedFlagName), "Error should be ErrReservedFlagName")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "identity", "Error details should mention the identity flag")
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
func TestGetGlobalFlagNames(t *testing.T) {
	// Create a test kit to ensure clean RootCmd state with global flags registered.
	_ = NewTestKit(t)

	// Get the reserved flag names.
	reserved := getGlobalFlagNames()

	// Log all reserved flags for debugging.
	t.Logf("Reserved flags: %v", reserved)

	// Verify that known global flags are in the reserved set.
	assert.True(t, reserved["chdir"], "chdir should be reserved")
	assert.True(t, reserved["C"], "C (chdir shorthand) should be reserved")
	assert.True(t, reserved["mask"], "mask should be reserved")
	assert.True(t, reserved["identity"], "identity should be reserved")
	assert.True(t, reserved["no-color"], "no-color should be reserved")

	// Verify that random names are not reserved.
	assert.False(t, reserved["my-custom-flag"], "my-custom-flag should NOT be reserved")
	assert.False(t, reserved["xyz"], "xyz should NOT be reserved")
}

// TestCustomCommand_NestedFlagConflictWithParent tests that nested custom commands
// cannot define flags that conflict with their parent custom command's persistent flags.
// This addresses the CodeRabbit review comment about validating flags at each recursion level.
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
		// Define a nested child command that tries to redefine the same flag.
		Commands: []schema.Command{
			{
				Name:        "child",
				Description: "Child command trying to redefine parent's flag",
				Flags: []schema.CommandFlag{
					{
						Name:  "parent-flag", // Conflicts with parent's --parent-flag.
						Type:  "bool",
						Usage: "This conflicts with parent's flag",
					},
				},
				Steps: stepsFromStrings("echo child"),
			},
		},
		Steps: stepsFromStrings("echo parent"),
	}

	// Add the test command to the config.
	atmosConfig.Commands = []schema.Command{parentCommand}

	// Process custom commands - should return error for the nested conflict.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)

	// Verify the error is returned correctly.
	require.Error(t, err, "Should return error for nested flag conflict with parent")
	assert.True(t, errors.Is(err, errUtils.ErrReservedFlagName), "Error should be ErrReservedFlagName")

	// Get the explanation from the error details.
	details := cockerrors.GetAllDetails(err)
	detailsStr := strings.Join(details, " ")
	assert.Contains(t, detailsStr, "parent-flag", "Error details should mention the conflicting flag name")
	assert.Contains(t, detailsStr, "child", "Error details should mention the child command name")
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
	require.Error(t, err, "Should return error for nested shorthand conflict with parent")
	assert.True(t, errors.Is(err, errUtils.ErrReservedFlagName), "Error should be ErrReservedFlagName")

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

// TestGetReservedFlagNamesFor tests that getReservedFlagNamesFor correctly returns
// flags from both the command itself and its ancestors.
func TestGetReservedFlagNamesFor(t *testing.T) {
	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Create a parent command with some persistent flags.
	parentCmd := &cobra.Command{
		Use:   "parent",
		Short: "Parent command",
	}
	parentCmd.PersistentFlags().String("parent-option", "", "Parent's option")
	parentCmd.PersistentFlags().StringP("another-option", "a", "", "Another option")

	// Add parent to RootCmd so it inherits global flags.
	RootCmd.AddCommand(parentCmd)
	defer RootCmd.RemoveCommand(parentCmd)

	// Get reserved flags for the parent command.
	reserved := getReservedFlagNamesFor(parentCmd)

	// Should include parent's own persistent flags.
	assert.True(t, reserved["parent-option"], "parent-option should be reserved")
	assert.True(t, reserved["another-option"], "another-option should be reserved")
	assert.True(t, reserved["a"], "shorthand 'a' should be reserved")

	// Should include inherited global flags from RootCmd.
	assert.True(t, reserved["chdir"], "chdir (inherited) should be reserved")
	assert.True(t, reserved["C"], "C (inherited shorthand) should be reserved")
	assert.True(t, reserved["mask"], "mask (inherited) should be reserved")

	// Should include the hardcoded identity flag.
	assert.True(t, reserved["identity"], "identity should be reserved")

	// Should NOT include random names.
	assert.False(t, reserved["random-flag"], "random-flag should NOT be reserved")
}
