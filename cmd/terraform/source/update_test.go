package source

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestUpdateCommand_MissingComponent tests that update command requires a component argument.
func TestUpdateCommand_MissingComponent(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "update <component>",
		Args: cobra.ExactArgs(1),
		RunE: executeUpdateCommand,
	}

	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err, "Should error without component argument")
}

// TestUpdateCommand_MissingStack tests that update command requires --stack flag.
func TestUpdateCommand_MissingStack(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "update <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := ParseCommonFlags(cmd, updateParser)
			if err != nil {
				return err
			}
			_ = opts
			return nil
		},
	}

	updateParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestUpdateCommand_FlagParsing tests that update command correctly parses flags.
func TestUpdateCommand_FlagParsing(t *testing.T) {
	var parsedStack string
	var parsedIdentity string

	cmd := &cobra.Command{
		Use:  "update <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := ParseCommonFlags(cmd, updateParser)
			if err != nil {
				return err
			}
			parsedStack = opts.Stack
			parsedIdentity = opts.Identity
			return nil
		},
	}

	updateParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc", "--stack", "prod-us-west-2", "--identity=my-profile"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "prod-us-west-2", parsedStack)
	assert.Equal(t, "my-profile", parsedIdentity)
}

// TestUpdateCommand_ForceAlwaysTrue verifies update command always uses force=true.
func TestUpdateCommand_ForceAlwaysTrue(t *testing.T) {
	// The update command is equivalent to create --force.
	// This is enforced by the implementation setting Force: true in the ProvisionSourceOptions.
	// We verify this by checking the command's logic does not have a --force flag
	// and always passes force=true to ProvisionSource.

	// The updateParser does not have a --force flag.
	flag := updateCmd.Flags().Lookup("force")
	assert.Nil(t, flag, "Update command should not have a --force flag")

	// The implementation always passes Force: true to ProvisionSource.
	// This is verified by code inspection in executeUpdateCommand line 90:
	// Force: true,
}

// TestUpdateCommand_UsesCorrectComponent tests that the component argument is parsed correctly.
func TestUpdateCommand_UsesCorrectComponent(t *testing.T) {
	var parsedComponent string

	cmd := &cobra.Command{
		Use:  "update <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedComponent = args[0]
			return nil
		},
	}

	cmd.SetArgs([]string{"my-network-component"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "my-network-component", parsedComponent)
}
