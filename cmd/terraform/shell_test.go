package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestShellCommandSetup verifies that the shell command is properly configured.
func TestShellCommandSetup(t *testing.T) {
	// Verify command is registered.
	require.NotNil(t, shellCmd)

	// Verify it's attached to terraformCmd.
	found := false
	for _, cmd := range terraformCmd.Commands() {
		if cmd.Name() == "shell" {
			found = true
			break
		}
	}
	assert.True(t, found, "shell should be registered as a subcommand of terraformCmd")

	// Verify command short and long descriptions.
	assert.Contains(t, shellCmd.Short, "environment")
	assert.Contains(t, shellCmd.Long, "Terraform")
}

// TestShellParserSetup verifies that the shell parser is properly configured.
func TestShellParserSetup(t *testing.T) {
	require.NotNil(t, shellParser, "shellParser should be initialized")

	// Verify the parser has the shell-specific flags.
	registry := shellParser.Registry()

	expectedFlags := []string{
		"process-templates",
		"process-functions",
		"skip",
	}

	for _, flagName := range expectedFlags {
		assert.True(t, registry.Has(flagName), "shellParser should have %s flag registered", flagName)
	}
}

// TestShellFlagDefaults verifies that shell command flags have correct default values.
func TestShellFlagDefaults(t *testing.T) {
	v := viper.New()

	// Bind parser to fresh viper instance.
	err := shellParser.BindToViper(v)
	require.NoError(t, err)

	// Verify default values.
	assert.True(t, v.GetBool("process-templates"), "process-templates should default to true")
	assert.True(t, v.GetBool("process-functions"), "process-functions should default to true")
}

// TestShellValidation tests validation errors in the shell command.
func TestShellValidation(t *testing.T) {
	t.Run("missing component returns error", func(t *testing.T) {
		// Reset viper to avoid state pollution.
		v := viper.New()
		v.Set("stack", "test-stack")

		// Bind the parser to the fresh viper instance.
		err := shellParser.BindToViper(v)
		require.NoError(t, err)

		// Create a test command to execute RunE directly.
		cmd := &cobra.Command{Use: "shell"}
		shellParser.RegisterFlags(cmd)

		// Execute RunE with no component argument.
		// In non-TTY environment, the prompt returns ErrInteractiveModeNotAvailable
		// which is swallowed, leaving component empty and triggering ErrMissingComponent.
		err = shellCmd.RunE(cmd, []string{})
		assert.ErrorIs(t, err, errUtils.ErrMissingComponent)
	})
}

// TestShellCommandArgs verifies that shell command accepts the correct number of arguments.
func TestShellCommandArgs(t *testing.T) {
	// The command should accept 0 or 1 argument (component name is optional).
	require.NotNil(t, shellCmd.Args)

	// Verify with no args.
	err := shellCmd.Args(shellCmd, []string{})
	assert.NoError(t, err, "shell command should accept 0 arguments")

	// Verify with one arg.
	err = shellCmd.Args(shellCmd, []string{"my-component"})
	assert.NoError(t, err, "shell command should accept 1 argument")

	// Verify with two args (should fail).
	err = shellCmd.Args(shellCmd, []string{"arg1", "arg2"})
	assert.Error(t, err, "shell command should reject more than 1 argument")
}

// TestShellCommandUsage verifies the command usage string.
func TestShellCommandUsage(t *testing.T) {
	assert.Equal(t, "shell [component]", shellCmd.Use)
}
