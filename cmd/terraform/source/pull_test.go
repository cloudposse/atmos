package source

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestPullCommand_MissingComponent tests that pull command requires a component argument.
func TestPullCommand_MissingComponent(t *testing.T) {
	// Create a new command instance to avoid shared state.
	cmd := &cobra.Command{
		Use:  "pull <component>",
		Args: cobra.ExactArgs(1),
		RunE: executePullCommand,
	}

	// Set up with no arguments.
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err, "Should error without component argument")
}

// TestPullCommand_MissingStack tests that pull command requires --stack flag.
func TestPullCommand_MissingStack(t *testing.T) {
	// Create a new command instance.
	cmd := &cobra.Command{
		Use:  "pull <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse common flags.
			opts, err := ParseCommonFlags(cmd, pullParser)
			if err != nil {
				return err
			}
			_ = opts
			return nil
		},
	}

	// Register flags.
	pullParser.RegisterFlags(cmd)

	// Set component but no --stack.
	cmd.SetArgs([]string{"vpc"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestPullCommand_FlagParsing tests that pull command correctly parses flags.
func TestPullCommand_FlagParsing(t *testing.T) {
	var parsedStack string
	var parsedForce bool

	// Create a command that captures parsed values.
	cmd := &cobra.Command{
		Use:  "pull <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := ParseCommonFlags(cmd, pullParser)
			if err != nil {
				return err
			}
			parsedStack = opts.Stack
			parsedForce = opts.Force
			return nil
		},
	}

	pullParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc", "--stack", "dev-us-east-1", "--force"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "dev-us-east-1", parsedStack)
	assert.True(t, parsedForce)
}

// TestPullCommand_UsesCorrectComponent tests that the component argument is parsed correctly.
func TestPullCommand_UsesCorrectComponent(t *testing.T) {
	var parsedComponent string

	cmd := &cobra.Command{
		Use:  "pull <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedComponent = args[0]
			return nil
		},
	}

	cmd.SetArgs([]string{"my-vpc-component"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "my-vpc-component", parsedComponent)
}

// TestPullCommand_ForceFlag tests that the --force flag works correctly.
func TestPullCommand_ForceFlag(t *testing.T) {
	var parsedForce bool

	cmd := &cobra.Command{
		Use:  "pull <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := ParseCommonFlags(cmd, pullParser)
			if err != nil {
				return err
			}
			parsedForce = opts.Force
			return nil
		},
	}

	pullParser.RegisterFlags(cmd)

	// Test without --force.
	cmd.SetArgs([]string{"vpc", "--stack", "dev"})
	err := cmd.Execute()
	require.NoError(t, err)
	assert.False(t, parsedForce, "Force should be false by default")

	// Test with --force.
	parsedForce = false // Reset.
	cmd.SetArgs([]string{"vpc", "--stack", "dev", "--force"})
	err = cmd.Execute()
	require.NoError(t, err)
	assert.True(t, parsedForce, "Force should be true when --force is specified")
}
