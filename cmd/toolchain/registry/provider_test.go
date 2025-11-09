package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistryCommandProvider tests RegistryCommandProvider implementation.
func TestRegistryCommandProvider(t *testing.T) {
	provider := &RegistryCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "registry", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "registry", provider.GetName())
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

	t.Run("Command has subcommands", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.True(t, cmd.HasSubCommands(), "registry command should have subcommands")
	})
}

// TestListCommandProvider tests ListCommandProvider implementation.
func TestListCommandProvider(t *testing.T) {
	provider := &ListCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, "list")
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "list", provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		require.NotNil(t, builder, "list command has flags and should return parser")
		assert.Equal(t, listParser, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
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
		assert.Equal(t, searchParser, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// TestListCommandHasFlags verifies that the list command has expected flags.
func TestListCommandHasFlags(t *testing.T) {
	expectedFlags := []string{"limit", "offset", "format", "sort"}

	for _, flagName := range expectedFlags {
		t.Run("has flag "+flagName, func(t *testing.T) {
			flag := listCmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "list command should have --%s flag", flagName)
		})
	}
}

// TestSearchCommandHasFlags verifies that the search command has expected flags.
func TestSearchCommandHasFlags(t *testing.T) {
	expectedFlags := []string{"limit", "registry", "format", "installed-only", "available-only"}

	for _, flagName := range expectedFlags {
		t.Run("has flag "+flagName, func(t *testing.T) {
			flag := searchCmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "search command should have --%s flag", flagName)
		})
	}
}

// TestRegistrySubcommands verifies registry has correct subcommands.
func TestRegistrySubcommands(t *testing.T) {
	provider := &RegistryCommandProvider{}
	cmd := provider.GetCommand()

	expectedSubcommands := []string{"list", "search"}

	for _, subName := range expectedSubcommands {
		t.Run("has subcommand "+subName, func(t *testing.T) {
			subCmd, _, err := cmd.Find([]string{subName})
			require.NoError(t, err)
			assert.Equal(t, subName, subCmd.Name())
		})
	}
}
