package source

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestDescribeCommand_MissingComponent tests that describe command requires a component argument.
func TestDescribeCommand_MissingComponent(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "describe <component>",
		Args: cobra.ExactArgs(1),
		RunE: executeDescribeCommand,
	}

	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err, "Should error without component argument")
}

// TestDescribeCommand_MissingStack tests that describe command requires --stack flag.
func TestDescribeCommand_MissingStack(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "describe <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()
			if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
				return err
			}
			stack := v.GetString("stack")
			if stack == "" {
				return errUtils.ErrRequiredFlagNotProvided
			}
			return nil
		},
	}

	describeParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestDescribeCommand_FlagParsing tests that describe command correctly parses flags.
func TestDescribeCommand_FlagParsing(t *testing.T) {
	var parsedStack string

	cmd := &cobra.Command{
		Use:  "describe <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()
			if err := describeParser.BindFlagsToViper(cmd, v); err != nil {
				return err
			}
			parsedStack = v.GetString("stack")
			if parsedStack == "" {
				return errUtils.ErrRequiredFlagNotProvided
			}
			return nil
		},
	}

	describeParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc", "--stack", "staging-eu-west-1"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "staging-eu-west-1", parsedStack)
}

// TestDescribeCommand_UsesCorrectComponent tests that the component argument is parsed correctly.
func TestDescribeCommand_UsesCorrectComponent(t *testing.T) {
	var parsedComponent string

	cmd := &cobra.Command{
		Use:  "describe <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedComponent = args[0]
			return nil
		},
	}

	cmd.SetArgs([]string{"my-rds-component"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "my-rds-component", parsedComponent)
}
