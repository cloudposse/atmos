package toolchain

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddCommand_Structure tests the add command structure.
func TestAddCommand_Structure(t *testing.T) {
	t.Run("command has correct use string with variadic args", func(t *testing.T) {
		assert.Contains(t, addCmd.Use, "add")
		assert.Contains(t, addCmd.Use, "<tool[@version]>")
		assert.Contains(t, addCmd.Use, "...")
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, addCmd.Short)
		assert.Contains(t, addCmd.Short, "Add")
	})

	t.Run("command has long description mentioning latest default", func(t *testing.T) {
		assert.NotEmpty(t, addCmd.Long)
		assert.Contains(t, addCmd.Long, "latest")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, addCmd.RunE)
	})
}

// TestAddCommand_Args tests argument handling for the add command.
func TestAddCommand_Args(t *testing.T) {
	t.Run("command accepts minimum 1 argument", func(t *testing.T) {
		// The Args function should be cobra.MinimumNArgs(1).
		require.NotNil(t, addCmd.Args)

		// Test that it fails with 0 args.
		err := addCmd.Args(addCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("command accepts 1 argument", func(t *testing.T) {
		err := addCmd.Args(addCmd, []string{"terraform@1.5.0"})
		assert.NoError(t, err)
	})

	t.Run("command accepts multiple arguments", func(t *testing.T) {
		err := addCmd.Args(addCmd, []string{"terraform@1.5.0", "kubectl@1.28.0", "opentofu@1.7.0"})
		assert.NoError(t, err)
	})
}

// TestAddCommandProvider_Extended tests additional AddCommandProvider functionality.
func TestAddCommandProvider_Extended(t *testing.T) {
	provider := &AddCommandProvider{}

	t.Run("command has correct Use string", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "add")
		assert.Contains(t, cmd.Use, "<tool[@version]>")
	})

	t.Run("provider returns correct command type", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.IsType(t, &cobra.Command{}, cmd)
	})
}

// TestAddCommand_MultiPackageSupport tests that the command supports multiple packages.
func TestAddCommand_MultiPackageSupport(t *testing.T) {
	t.Run("Use string indicates variadic arguments", func(t *testing.T) {
		// The ... suffix indicates variadic arguments.
		assert.Contains(t, addCmd.Use, "...")
	})

	t.Run("Args validator allows multiple arguments", func(t *testing.T) {
		// Test with various numbers of arguments.
		testCases := []struct {
			name    string
			args    []string
			wantErr bool
		}{
			{"zero args", []string{}, true},
			{"one arg", []string{"terraform@1.5.0"}, false},
			{"two args", []string{"terraform@1.5.0", "kubectl@1.28.0"}, false},
			{"three args", []string{"a@1", "b@2", "c@3"}, false},
			{"five args", []string{"a@1", "b@2", "c@3", "d@4", "e@5"}, false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := addCmd.Args(addCmd, tc.args)
				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// TestAddCommand_LatestVersionDefault tests that the Long description documents the latest default.
func TestAddCommand_LatestVersionDefault(t *testing.T) {
	t.Run("Long description mentions version defaults to latest", func(t *testing.T) {
		assert.Contains(t, addCmd.Long, "latest")
		assert.Contains(t, addCmd.Long, "omitted")
	})

	t.Run("Use string shows version is optional with brackets", func(t *testing.T) {
		// The [@version] syntax indicates version is optional.
		assert.Contains(t, addCmd.Use, "[@version]")
	})
}
