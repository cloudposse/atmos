package toolchain

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// TestCommandProviderImplementations verifies that all toolchain subcommands
// implement the CommandProvider interface correctly.
func TestCommandProviderImplementations(t *testing.T) {
	tests := []struct {
		name              string
		providerName      string
		commandName       string
		group             string
		expectFlagsParser bool
		getFlagsBuilder   func() flags.Builder
	}{
		{
			name:              "AddCommandProvider",
			providerName:      "add",
			commandName:       "add",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&AddCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "CleanCommandProvider",
			providerName:      "clean",
			commandName:       "clean",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&CleanCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "ExecCommandProvider",
			providerName:      "exec",
			commandName:       "exec",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&ExecCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "GetCommandProvider",
			providerName:      "get",
			commandName:       "get",
			group:             "Toolchain Commands",
			expectFlagsParser: true,
			getFlagsBuilder:   func() flags.Builder { return (&GetCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "InfoCommandProvider",
			providerName:      "info",
			commandName:       "info",
			group:             "Toolchain Commands",
			expectFlagsParser: true,
			getFlagsBuilder:   func() flags.Builder { return (&InfoCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "InstallCommandProvider",
			providerName:      "install",
			commandName:       "install",
			group:             "Toolchain Commands",
			expectFlagsParser: true,
			getFlagsBuilder:   func() flags.Builder { return (&InstallCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "ListCommandProvider",
			providerName:      "list",
			commandName:       "list",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&ListCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "PathCommandProvider",
			providerName:      "path",
			commandName:       "path",
			group:             "Toolchain Commands",
			expectFlagsParser: true,
			getFlagsBuilder:   func() flags.Builder { return (&PathCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "RemoveCommandProvider",
			providerName:      "remove",
			commandName:       "remove",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&RemoveCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "SearchCommandProvider",
			providerName:      "search",
			commandName:       "search",
			group:             "Toolchain Commands",
			expectFlagsParser: true,
			getFlagsBuilder:   func() flags.Builder { return (&SearchCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "SetCommandProvider",
			providerName:      "set",
			commandName:       "set",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&SetCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "UninstallCommandProvider",
			providerName:      "uninstall",
			commandName:       "uninstall",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&UninstallCommandProvider{}).GetFlagsBuilder() },
		},
		{
			name:              "WhichCommandProvider",
			providerName:      "which",
			commandName:       "which",
			group:             "Toolchain Commands",
			expectFlagsParser: false,
			getFlagsBuilder:   func() flags.Builder { return (&WhichCommandProvider{}).GetFlagsBuilder() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test GetFlagsBuilder.
			builder := tt.getFlagsBuilder()
			if tt.expectFlagsParser {
				assert.NotNil(t, builder, "Expected flags builder to be non-nil for %s", tt.name)
			} else {
				assert.Nil(t, builder, "Expected flags builder to be nil for %s", tt.name)
			}
		})
	}
}

// TestAddCommandProvider tests AddCommandProvider implementation.
func TestAddCommandProvider(t *testing.T) {
	provider := &AddCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "add", cmd.Use[:3])
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "add", provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// TestCleanCommandProvider tests CleanCommandProvider implementation.
func TestCleanCommandProvider(t *testing.T) {
	provider := &CleanCommandProvider{}
	testBasicCommandProvider(t, provider, "clean", "clean")
}

// TestExecCommandProvider tests ExecCommandProvider implementation.
func TestExecCommandProvider(t *testing.T) {
	provider := &ExecCommandProvider{}
	testBasicCommandProvider(t, provider, "exec", "exec")
}

// testBasicCommandProvider is a helper to test basic command provider implementations without flags.
func testBasicCommandProvider(t *testing.T, provider interface{}, expectedName, expectedCommandUse string) {
	t.Helper()

	// Type assert to get the necessary interfaces.
	type commandGetter interface {
		GetCommand() *cobra.Command
	}
	type nameGetter interface {
		GetName() string
	}
	type groupGetter interface {
		GetGroup() string
	}
	type posArgGetter interface {
		GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
	}
	type compatFlagGetter interface {
		GetCompatibilityFlags() map[string]compat.CompatibilityFlag
	}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		if p, ok := provider.(commandGetter); ok {
			cmd := p.GetCommand()
			require.NotNil(t, cmd)
			assert.Contains(t, cmd.Use, expectedCommandUse)
		}
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		if p, ok := provider.(nameGetter); ok {
			assert.Equal(t, expectedName, p.GetName())
		}
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		if p, ok := provider.(groupGetter); ok {
			assert.Equal(t, "Toolchain Commands", p.GetGroup())
		}
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		if p, ok := provider.(posArgGetter); ok {
			assert.Nil(t, p.GetPositionalArgsBuilder())
		}
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		if p, ok := provider.(compatFlagGetter); ok {
			assert.Nil(t, p.GetCompatibilityFlags())
		}
	})
}

// TestGetCommandProvider tests GetCommandProvider implementation.
func TestGetCommandProvider(t *testing.T) {
	provider := &GetCommandProvider{}
	testCommandProviderWithFlags(t, provider, "get", "get", getParser)
}

// TestInfoCommandProvider tests InfoCommandProvider implementation.
func TestInfoCommandProvider(t *testing.T) {
	provider := &InfoCommandProvider{}
	testCommandProviderWithFlags(t, provider, "info", "info", infoParser)
}

// TestInstallCommandProvider tests InstallCommandProvider implementation.
func TestInstallCommandProvider(t *testing.T) {
	provider := &InstallCommandProvider{}
	testCommandProviderWithFlags(t, provider, "install", "install", installParser)
}

// TestPathCommandProvider tests PathCommandProvider implementation.
func TestPathCommandProvider(t *testing.T) {
	provider := &PathCommandProvider{}
	testCommandProviderWithFlags(t, provider, "path", "path", pathParser)
}

// testCommandProviderWithFlags tests command providers that have flags.
func testCommandProviderWithFlags(t *testing.T, provider interface{}, expectedName, expectedCommandUse string, expectedParser flags.Builder) {
	t.Helper()

	// Type assert to get the necessary interfaces.
	type commandGetter interface {
		GetCommand() *cobra.Command
	}
	type nameGetter interface {
		GetName() string
	}
	type groupGetter interface {
		GetGroup() string
	}
	type flagsGetter interface {
		GetFlagsBuilder() flags.Builder
	}
	type posArgGetter interface {
		GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
	}
	type compatFlagGetter interface {
		GetCompatibilityFlags() map[string]compat.CompatibilityFlag
	}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		if p, ok := provider.(commandGetter); ok {
			cmd := p.GetCommand()
			require.NotNil(t, cmd)
			assert.Contains(t, cmd.Use, expectedCommandUse)
		}
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		if p, ok := provider.(nameGetter); ok {
			assert.Equal(t, expectedName, p.GetName())
		}
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		if p, ok := provider.(groupGetter); ok {
			assert.Equal(t, "Toolchain Commands", p.GetGroup())
		}
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		if p, ok := provider.(flagsGetter); ok {
			builder := p.GetFlagsBuilder()
			require.NotNil(t, builder, "command has flags and should return parser")
			assert.Equal(t, expectedParser, builder)
		}
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		if p, ok := provider.(posArgGetter); ok {
			assert.Nil(t, p.GetPositionalArgsBuilder())
		}
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		if p, ok := provider.(compatFlagGetter); ok {
			assert.Nil(t, p.GetCompatibilityFlags())
		}
	})
}

// TestListCommandProvider tests ListCommandProvider implementation.
func TestListCommandProvider(t *testing.T) {
	provider := &ListCommandProvider{}
	testBasicCommandProvider(t, provider, "list", "list")
}

// TestRemoveCommandProvider tests RemoveCommandProvider implementation.
func TestRemoveCommandProvider(t *testing.T) {
	provider := &RemoveCommandProvider{}
	testBasicCommandProvider(t, provider, "remove", "remove")
}

// TestSetCommandProvider tests SetCommandProvider implementation.
func TestSetCommandProvider(t *testing.T) {
	provider := &SetCommandProvider{}
	testBasicCommandProvider(t, provider, "set", "set")
}

// TestUninstallCommandProvider tests UninstallCommandProvider implementation.
func TestUninstallCommandProvider(t *testing.T) {
	provider := &UninstallCommandProvider{}
	testBasicCommandProvider(t, provider, "uninstall", "uninstall")
}

// TestWhichCommandProvider tests WhichCommandProvider implementation.
func TestWhichCommandProvider(t *testing.T) {
	provider := &WhichCommandProvider{}
	testBasicCommandProvider(t, provider, "which", "which")
}

// TestSearchCommandProvider tests SearchCommandProvider implementation.
func TestSearchCommandProvider(t *testing.T) {
	provider := &SearchCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "search")
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "search", provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		require.NotNil(t, builder, "search command has flags and should return parser")
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// TestToolchainCommandProvider tests ToolchainCommandProvider implementation.
func TestToolchainCommandProvider(t *testing.T) {
	provider := &ToolchainCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "toolchain", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "toolchain", provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		require.NotNil(t, builder, "toolchain command has persistent flags and should return parser")
		assert.Equal(t, toolchainParser, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("Command has subcommands", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.True(t, cmd.HasSubCommands(), "toolchain command should have subcommands")
	})
}
