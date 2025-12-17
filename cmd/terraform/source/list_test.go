package source

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestListCommand_MissingStack tests that list command requires --stack flag.
func TestListCommand_MissingStack(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()
			if err := listParser.BindFlagsToViper(cmd, v); err != nil {
				return err
			}
			stack := v.GetString("stack")
			if stack == "" {
				return errUtils.ErrRequiredFlagNotProvided
			}
			return nil
		},
	}

	listParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestListCommand_FlagParsing tests that list command correctly parses flags.
func TestListCommand_FlagParsing(t *testing.T) {
	var parsedStack string

	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()
			if err := listParser.BindFlagsToViper(cmd, v); err != nil {
				return err
			}
			parsedStack = v.GetString("stack")
			if parsedStack == "" {
				return errUtils.ErrRequiredFlagNotProvided
			}
			// Return early to skip the NotImplemented error for this test.
			return nil
		},
	}

	listParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"--stack", "dev-us-east-1"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "dev-us-east-1", parsedStack)
}

// TestListCommand_NotImplemented tests that list command returns ErrNotImplemented.
func TestListCommand_NotImplemented(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeListCommand(cmd)
		},
	}

	listParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"--stack", "dev"})
	err := cmd.Execute()

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
}

// TestListCommand_NoArgs tests that list command accepts no positional arguments.
func TestListCommand_NoArgs(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Just verify args validation passes.
			return nil
		},
	}

	// Test with extra arguments should fail.
	cmd.SetArgs([]string{"unexpected-arg"})
	err := cmd.Execute()

	require.Error(t, err, "Should error with unexpected positional arguments")
}
