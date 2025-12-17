package source

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestCreateCommand_MissingComponent tests that create command requires a component argument.
func TestCreateCommand_MissingComponent(t *testing.T) {
	// Create a new command instance to avoid shared state.
	cmd := &cobra.Command{
		Use:  "create <component>",
		Args: cobra.ExactArgs(1),
		RunE: executeCreateCommand,
	}

	// Set up with no arguments.
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err, "Should error without component argument")
}

// TestCreateCommand_MissingStack tests that create command requires --stack flag.
func TestCreateCommand_MissingStack(t *testing.T) {
	// Create a new command instance.
	cmd := &cobra.Command{
		Use:  "create <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse common flags.
			opts, err := ParseCommonFlags(cmd, createParser)
			if err != nil {
				return err
			}
			_ = opts
			return nil
		},
	}

	// Register flags.
	createParser.RegisterFlags(cmd)

	// Set component but no --stack.
	cmd.SetArgs([]string{"vpc"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestCreateCommand_FlagParsing tests that create command correctly parses flags.
func TestCreateCommand_FlagParsing(t *testing.T) {
	var parsedStack string
	var parsedForce bool

	// Create a command that captures parsed values.
	cmd := &cobra.Command{
		Use:  "create <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := ParseCommonFlags(cmd, createParser)
			if err != nil {
				return err
			}
			parsedStack = opts.Stack
			parsedForce = opts.Force
			return nil
		},
	}

	createParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc", "--stack", "dev-us-east-1", "--force"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "dev-us-east-1", parsedStack)
	assert.True(t, parsedForce)
}

// TestCreateCommand_UsesCorrectComponent tests that the component argument is parsed correctly.
func TestCreateCommand_UsesCorrectComponent(t *testing.T) {
	var parsedComponent string

	cmd := &cobra.Command{
		Use:  "create <component>",
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
