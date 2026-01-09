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
