package registry

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
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

// commandProviderTestCase defines a test case for command provider testing.
type commandProviderTestCase struct {
	name                 string
	provider             interface{}
	expectedCommandUse   string
	expectedName         string
	expectedGroup        string
	expectFlagsBuilder   bool
	expectedFlagsBuilder interface{}
}

// testCommandProvider is a helper to test command provider implementations.
func testCommandProvider(t *testing.T, tc *commandProviderTestCase) {
	t.Helper()

	// Use type assertion to access provider methods.
	type providerInterface interface {
		GetCommand() *cobra.Command
		GetName() string
		GetGroup() string
		GetFlagsBuilder() flags.Builder
		GetPositionalArgsBuilder() *flags.PositionalArgsBuilder
		GetCompatibilityFlags() map[string]compat.CompatibilityFlag
	}

	provider, ok := tc.provider.(providerInterface)
	require.True(t, ok, "provider must implement providerInterface")

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Contains(t, cmd.Use, tc.expectedCommandUse)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, tc.expectedName, provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, tc.expectedGroup, provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns expected parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		if tc.expectFlagsBuilder {
			require.NotNil(t, builder, "%s command has flags and should return parser", tc.expectedName)
			assert.Equal(t, tc.expectedFlagsBuilder, builder)
		} else {
			assert.Nil(t, builder)
		}
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

// TestListCommandProvider tests ListCommandProvider implementation.
func TestListCommandProvider(t *testing.T) {
	testCommandProvider(t, &commandProviderTestCase{
		name:                 "list",
		provider:             &ListCommandProvider{},
		expectedCommandUse:   "list",
		expectedName:         "list",
		expectedGroup:        "Toolchain Commands",
		expectFlagsBuilder:   true,
		expectedFlagsBuilder: listParser,
	})
}

// TestSearchCommandProvider tests SearchCommandProvider implementation.
func TestSearchCommandProvider(t *testing.T) {
	testCommandProvider(t, &commandProviderTestCase{
		name:                 "search",
		provider:             &SearchCommandProvider{},
		expectedCommandUse:   "search",
		expectedName:         "search",
		expectedGroup:        "Toolchain Commands",
		expectFlagsBuilder:   true,
		expectedFlagsBuilder: searchParser,
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

// TestGetRegistryCommand tests the GetRegistryCommand function.
func TestGetRegistryCommand(t *testing.T) {
	cmd := GetRegistryCommand()
	require.NotNil(t, cmd, "GetRegistryCommand should return non-nil command")
	assert.Equal(t, "registry", cmd.Use, "GetRegistryCommand should return registry command")
}

// TestListConfiguredRegistries tests listConfiguredRegistries function.
func TestListConfiguredRegistries(t *testing.T) {
	// This function just outputs a message - test that it doesn't error.
	err := listConfiguredRegistries(context.Background())
	assert.NoError(t, err, "listConfiguredRegistries should not return error")
}

// TestBuildToolsTable tests buildToolsTable function with various inputs.
func TestBuildToolsTable(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
	}{
		{
			name:  "empty tools list",
			tools: []*toolchainregistry.Tool{},
		},
		{
			name: "single tool",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "hashicorp",
					RepoName:  "terraform",
					Type:      "github_release",
				},
			},
		},
		{
			name: "multiple tools",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "hashicorp",
					RepoName:  "terraform",
					Type:      "github_release",
				},
				{
					RepoOwner: "kubernetes",
					RepoName:  "kubectl",
					Type:      "github_release",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function just outputs UI - test that it doesn't panic.
			assert.NotPanics(t, func() {
				buildToolsTable(tt.tools)
			}, "buildToolsTable should not panic")
		})
	}
}

// TestDisplaySearchResults tests displaySearchResults function with various inputs.
func TestDisplaySearchResults(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
	}{
		{
			name:  "empty results",
			tools: []*toolchainregistry.Tool{},
		},
		{
			name: "single result",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "hashicorp",
					RepoName:  "terraform",
					Type:      "github_release",
					Registry:  "aqua-public",
				},
			},
		},
		{
			name: "multiple results",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "hashicorp",
					RepoName:  "terraform",
					Type:      "github_release",
					Registry:  "aqua-public",
				},
				{
					RepoOwner: "kubernetes",
					RepoName:  "kubectl",
					Type:      "github_release",
					Registry:  "aqua-public",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function just outputs UI - test that it doesn't panic.
			assert.NotPanics(t, func() {
				displaySearchResults(tt.tools)
			}, "displaySearchResults should not panic")
		})
	}
}
