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
