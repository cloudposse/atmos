package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDeleteCommand_MissingComponent tests that delete command requires a component argument.
func TestDeleteCommand_MissingComponent(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "delete <component>",
		Args: cobra.ExactArgs(1),
		RunE: executeDeleteCommand,
	}

	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err, "Should error without component argument")
}

// TestDeleteCommand_MissingStack tests that delete command requires --stack flag.
func TestDeleteCommand_MissingStack(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "delete <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := parseDeleteFlags(cmd)
			return err
		},
	}

	deleteParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --stack flag")
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestDeleteCommand_MissingForce tests that delete command requires --force flag.
func TestDeleteCommand_MissingForce(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "delete <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := parseDeleteFlags(cmd)
			return err
		},
	}

	deleteParser.RegisterFlags(cmd)

	// Provide --stack but not --force.
	cmd.SetArgs([]string{"vpc", "--stack", "dev"})
	err := cmd.Execute()

	require.Error(t, err, "Should error without --force flag")
	assert.ErrorIs(t, err, errUtils.ErrForceRequired)
}

// TestDeleteCommand_FlagParsing tests that delete command correctly parses flags.
func TestDeleteCommand_FlagParsing(t *testing.T) {
	var parsedStack string

	cmd := &cobra.Command{
		Use:  "delete <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := parseDeleteFlags(cmd)
			if err != nil {
				return err
			}
			parsedStack = opts.Stack
			return nil
		},
	}

	deleteParser.RegisterFlags(cmd)

	cmd.SetArgs([]string{"vpc", "--stack", "prod-us-west-2", "--force"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "prod-us-west-2", parsedStack)
}

// TestDeleteCommand_UsesCorrectComponent tests that the component argument is parsed correctly.
func TestDeleteCommand_UsesCorrectComponent(t *testing.T) {
	var parsedComponent string

	cmd := &cobra.Command{
		Use:  "delete <component>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedComponent = args[0]
			return nil
		},
	}

	cmd.SetArgs([]string{"my-component-to-delete"})
	err := cmd.Execute()

	require.NoError(t, err)
	assert.Equal(t, "my-component-to-delete", parsedComponent)
}

// TestDeleteSourceDirectory_DirectoryNotExists tests that delete handles non-existent directory.
func TestDeleteSourceDirectory_DirectoryNotExists(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: tempDir,
			},
		},
	}

	componentConfig := map[string]any{}

	// Directory doesn't exist - should not error, just warn.
	err := deleteSourceDirectory(atmosConfig, "nonexistent-vpc", componentConfig, "dev")
	assert.NoError(t, err, "Should not error when directory doesn't exist")
}

// TestDeleteSourceDirectory_Success tests successful directory deletion.
func TestDeleteSourceDirectory_Success(t *testing.T) {
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "vpc")

	// Create the directory to be deleted.
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	// Verify directory exists.
	_, err = os.Stat(componentDir)
	require.NoError(t, err, "Directory should exist before delete")

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: tempDir,
			},
		},
	}

	componentConfig := map[string]any{}

	err = deleteSourceDirectory(atmosConfig, "vpc", componentConfig, "dev")
	require.NoError(t, err)

	// Verify directory was deleted.
	_, err = os.Stat(componentDir)
	assert.True(t, os.IsNotExist(err), "Directory should be deleted")
}

// TestDeleteSourceDirectory_TargetDirectoryError tests error when target directory cannot be determined.
func TestDeleteSourceDirectory_TargetDirectoryError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "", // Empty base path should cause error.
			},
		},
	}

	componentConfig := map[string]any{}

	err := deleteSourceDirectory(atmosConfig, "vpc", componentConfig, "dev")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceProvision)
}
