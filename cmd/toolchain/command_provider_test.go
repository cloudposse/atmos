package toolchain

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// CommandProvider interface for testing.
type testableCommandProvider interface {
	GetCommand() *cobra.Command
	GetName() string
	GetGroup() string
	GetFlagsBuilder() flags.Builder
	GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
	GetCompatibilityFlags() map[string]compat.CompatibilityFlag
}

// testCommandProvider runs standard tests for any CommandProvider implementation.
func testCommandProvider(t *testing.T, provider testableCommandProvider, expectedName string, expectedCmd *cobra.Command, expectFlagsBuilder bool) {
	t.Helper()

	t.Run("GetCommand returns non-nil", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, expectedCmd, cmd)
	})

	t.Run("GetName returns expected name", func(t *testing.T) {
		assert.Equal(t, expectedName, provider.GetName())
	})

	t.Run("GetGroup returns Toolchain Commands", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder", func(t *testing.T) {
		if expectFlagsBuilder {
			assert.NotNil(t, provider.GetFlagsBuilder())
		} else {
			assert.Nil(t, provider.GetFlagsBuilder())
		}
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// testCommandArgsValidation runs standard Args validation tests for any command.
func testCommandArgsValidation(t *testing.T, cmd *cobra.Command, expectNoArgs bool, minArgs, maxArgs int) {
	t.Helper()

	if expectNoArgs {
		t.Run("Args is set to NoArgs", func(t *testing.T) {
			require.NotNil(t, cmd.Args, "Args should be set")
		})

		t.Run("accepts zero arguments", func(t *testing.T) {
			err := cmd.Args(cmd, []string{})
			assert.NoError(t, err)
		})

		t.Run("rejects unexpected arguments", func(t *testing.T) {
			err := cmd.Args(cmd, []string{"unexpected"})
			assert.Error(t, err)
		})
	} else {
		t.Run("Args is set", func(t *testing.T) {
			require.NotNil(t, cmd.Args, "Args should be set")
		})

		if minArgs == 0 {
			t.Run("accepts zero arguments", func(t *testing.T) {
				err := cmd.Args(cmd, []string{})
				assert.NoError(t, err)
			})
		}

		if maxArgs > 0 {
			t.Run("accepts valid number of arguments", func(t *testing.T) {
				args := make([]string, maxArgs)
				for i := range args {
					args[i] = "arg"
				}
				err := cmd.Args(cmd, args)
				assert.NoError(t, err)
			})
		}
	}
}

// testCommandStructure runs standard structure tests for any command.
func testCommandStructure(t *testing.T, cmd *cobra.Command, expectedUse string, shortContains, longContains string) {
	t.Helper()

	t.Run("Use is correct", func(t *testing.T) {
		assert.Equal(t, expectedUse, cmd.Use)
	})

	t.Run("Short description is set", func(t *testing.T) {
		assert.NotEmpty(t, cmd.Short)
		if shortContains != "" {
			assert.Contains(t, cmd.Short, shortContains)
		}
	})

	t.Run("Long description is set", func(t *testing.T) {
		assert.NotEmpty(t, cmd.Long)
		if longContains != "" {
			assert.Contains(t, cmd.Long, longContains)
		}
	})

	t.Run("RunE is set", func(t *testing.T) {
		assert.NotNil(t, cmd.RunE)
	})
}

// TestAllCommandProviders tests all command providers in a table-driven way.
func TestAllCommandProviders(t *testing.T) {
	tests := []struct {
		name               string
		provider           testableCommandProvider
		expectedName       string
		expectedCmd        *cobra.Command
		expectFlagsBuilder bool
	}{
		{
			name:               "ListCommandProvider",
			provider:           &ListCommandProvider{},
			expectedName:       "list",
			expectedCmd:        listCmd,
			expectFlagsBuilder: false,
		},
		{
			name:               "WhichCommandProvider",
			provider:           &WhichCommandProvider{},
			expectedName:       "which",
			expectedCmd:        whichCmd,
			expectFlagsBuilder: false,
		},
		{
			name:               "PathCommandProvider",
			provider:           &PathCommandProvider{},
			expectedName:       "path",
			expectedCmd:        pathCmd,
			expectFlagsBuilder: true,
		},
		{
			name:               "CleanCommandProvider",
			provider:           &CleanCommandProvider{},
			expectedName:       "clean",
			expectedCmd:        cleanCmd,
			expectFlagsBuilder: false,
		},
		{
			name:               "GetCommandProvider",
			provider:           &GetCommandProvider{},
			expectedName:       "get",
			expectedCmd:        getCmd,
			expectFlagsBuilder: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCommandProvider(t, tt.provider, tt.expectedName, tt.expectedCmd, tt.expectFlagsBuilder)
		})
	}
}

// TestAllCommandStructures tests command structures in a table-driven way.
func TestAllCommandStructures(t *testing.T) {
	tests := []struct {
		name          string
		cmd           *cobra.Command
		expectedUse   string
		shortContains string
		longContains  string
	}{
		{
			name:          "list command",
			cmd:           listCmd,
			expectedUse:   "list",
			shortContains: "List",
			longContains:  ".tool-versions",
		},
		{
			name:          "which command",
			cmd:           whichCmd,
			expectedUse:   "which <tool>",
			shortContains: "path",
			longContains:  "path",
		},
		{
			name:          "path command",
			cmd:           pathCmd,
			expectedUse:   "path",
			shortContains: "alias",
			longContains:  ".tool-versions",
		},
		{
			name:          "clean command",
			cmd:           cleanCmd,
			expectedUse:   "clean",
			shortContains: "Clean",
			longContains:  "Remove",
		},
		{
			name:          "get command",
			cmd:           getCmd,
			expectedUse:   "get [tool]",
			shortContains: "version",
			longContains:  ".tool-versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCommandStructure(t, tt.cmd, tt.expectedUse, tt.shortContains, tt.longContains)
		})
	}
}

// TestAllCommandArgsValidation tests Args validation for all commands.
func TestAllCommandArgsValidation(t *testing.T) {
	tests := []struct {
		name         string
		cmd          *cobra.Command
		expectNoArgs bool
		minArgs      int
		maxArgs      int
	}{
		{
			name:         "list command accepts no args",
			cmd:          listCmd,
			expectNoArgs: true,
		},
		{
			name:         "clean command accepts no args",
			cmd:          cleanCmd,
			expectNoArgs: true,
		},
		{
			name:         "path command accepts no args",
			cmd:          pathCmd,
			expectNoArgs: true,
		},
		{
			name:         "get command accepts zero or one arg",
			cmd:          getCmd,
			expectNoArgs: false,
			minArgs:      0,
			maxArgs:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testCommandArgsValidation(t, tt.cmd, tt.expectNoArgs, tt.minArgs, tt.maxArgs)
		})
	}
}
