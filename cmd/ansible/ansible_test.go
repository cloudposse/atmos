package ansible

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAnsibleCommandProvider(t *testing.T) {
	provider := &AnsibleCommandProvider{}

	t.Run("GetCommand returns the ansible command", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "ansible", cmd.Use)
		assert.Equal(t, []string{"an"}, cmd.Aliases)
	})

	t.Run("GetName returns ansible", func(t *testing.T) {
		assert.Equal(t, "ansible", provider.GetName())
	})

	t.Run("GetGroup returns Core Stack Commands", func(t *testing.T) {
		assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetAliases())
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

	t.Run("IsExperimental returns false", func(t *testing.T) {
		assert.False(t, provider.IsExperimental())
	})
}

func TestAnsibleCommandStructure(t *testing.T) {
	t.Run("ansible command has correct properties", func(t *testing.T) {
		assert.Equal(t, "ansible", ansibleCmd.Use)
		assert.Equal(t, []string{"an"}, ansibleCmd.Aliases)
		assert.Contains(t, ansibleCmd.Short, "ansible")
		assert.NotEmpty(t, ansibleCmd.Long)
	})

	t.Run("ansible command has FParseErrWhitelist for unknown flags", func(t *testing.T) {
		assert.True(t, ansibleCmd.FParseErrWhitelist.UnknownFlags)
	})

	t.Run("ansible command has subcommands", func(t *testing.T) {
		subcommands := ansibleCmd.Commands()
		require.GreaterOrEqual(t, len(subcommands), 2)

		// Check that playbook and version subcommands exist.
		subcommandNames := make([]string, len(subcommands))
		for i, cmd := range subcommands {
			subcommandNames[i] = cmd.Name()
		}
		assert.Contains(t, subcommandNames, "playbook")
		assert.Contains(t, subcommandNames, "version")
	})
}

func TestPlaybookCommandStructure(t *testing.T) {
	t.Run("playbook command has correct properties", func(t *testing.T) {
		assert.Equal(t, "playbook", playbookCmd.Use)
		assert.Equal(t, []string{"pb"}, playbookCmd.Aliases)
		assert.Contains(t, playbookCmd.Short, "Ansible playbook")
		assert.NotEmpty(t, playbookCmd.Long)
	})

	t.Run("playbook command has FParseErrWhitelist for unknown flags", func(t *testing.T) {
		assert.True(t, playbookCmd.FParseErrWhitelist.UnknownFlags)
	})
}

func TestVersionCommandStructure(t *testing.T) {
	t.Run("version command has correct properties", func(t *testing.T) {
		assert.Equal(t, "version", versionCmd.Use)
		assert.Contains(t, versionCmd.Short, "version")
		assert.NotEmpty(t, versionCmd.Long)
	})
}

func TestBuildConfigAndStacksInfo(t *testing.T) {
	t.Run("returns empty info when no stack flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		info := buildConfigAndStacksInfo(cmd)
		assert.Equal(t, schema.ConfigAndStacksInfo{}, info)
	})

	t.Run("returns info with stack when flag is set", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("stack", "", "stack name")
		err := cmd.Flags().Set("stack", "dev-us-east-1")
		require.NoError(t, err)

		info := buildConfigAndStacksInfo(cmd)
		assert.Equal(t, "dev-us-east-1", info.Stack)
	})
}

func TestProcessArgs(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		expectedComponent  string
		expectedAdditional []string
	}{
		{
			name:               "empty args",
			args:               []string{},
			expectedComponent:  "",
			expectedAdditional: nil,
		},
		{
			name:               "component only",
			args:               []string{"my-component"},
			expectedComponent:  "my-component",
			expectedAdditional: nil,
		},
		{
			name:               "component with additional args",
			args:               []string{"my-component", "--verbose", "--check"},
			expectedComponent:  "my-component",
			expectedAdditional: []string{"--verbose", "--check"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			component, additionalArgs := processArgs(tc.args)
			assert.Equal(t, tc.expectedComponent, component)
			assert.Equal(t, tc.expectedAdditional, additionalArgs)
		})
	}
}

func TestInitConfigAndStacksInfo(t *testing.T) {
	t.Run("initializes info with correct values", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("stack", "", "stack name")
		err := cmd.Flags().Set("stack", "prod-us-west-2")
		require.NoError(t, err)

		args := []string{"webserver", "--verbose"}

		info := initConfigAndStacksInfo(cmd, "playbook", args)

		assert.Equal(t, "ansible", info.ComponentType)
		assert.Equal(t, "playbook", info.SubCommand)
		assert.Equal(t, []string{"ansible", "playbook"}, info.CliArgs)
		assert.Equal(t, "webserver", info.ComponentFromArg)
		assert.Equal(t, []string{"--verbose"}, info.AdditionalArgsAndFlags)
		assert.Equal(t, "prod-us-west-2", info.Stack)
	})

	t.Run("handles empty args", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}

		info := initConfigAndStacksInfo(cmd, "version", []string{})

		assert.Equal(t, "ansible", info.ComponentType)
		assert.Equal(t, "version", info.SubCommand)
		assert.Equal(t, []string{"ansible", "version"}, info.CliArgs)
		assert.Empty(t, info.ComponentFromArg)
		assert.Empty(t, info.AdditionalArgsAndFlags)
	})
}

func TestGetAnsibleFlags(t *testing.T) {
	t.Run("extracts playbook flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("playbook", "", "playbook")
		cmd.Flags().String("inventory", "", "inventory")
		err := cmd.Flags().Set("playbook", "site.yml")
		require.NoError(t, err)

		flags := getAnsibleFlags(cmd)
		assert.Equal(t, "site.yml", flags.Playbook)
		assert.Empty(t, flags.Inventory)
	})

	t.Run("extracts inventory flag", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("playbook", "", "playbook")
		cmd.Flags().String("inventory", "", "inventory")
		err := cmd.Flags().Set("inventory", "hosts.ini")
		require.NoError(t, err)

		flags := getAnsibleFlags(cmd)
		assert.Empty(t, flags.Playbook)
		assert.Equal(t, "hosts.ini", flags.Inventory)
	})

	t.Run("extracts both flags", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("playbook", "", "playbook")
		cmd.Flags().String("inventory", "", "inventory")
		err := cmd.Flags().Set("playbook", "deploy.yml")
		require.NoError(t, err)
		err = cmd.Flags().Set("inventory", "production")
		require.NoError(t, err)

		flags := getAnsibleFlags(cmd)
		assert.Equal(t, "deploy.yml", flags.Playbook)
		assert.Equal(t, "production", flags.Inventory)
	})

	t.Run("handles missing flags gracefully", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		flags := getAnsibleFlags(cmd)
		assert.Empty(t, flags.Playbook)
		assert.Empty(t, flags.Inventory)
	})
}

func TestAnsibleGlobalFlagsHandler(t *testing.T) {
	t.Run("returns usage when called without subcommand", func(t *testing.T) {
		// ansibleGlobalFlagsHandler calls cmd.Usage() which returns nil.
		err := ansibleGlobalFlagsHandler(ansibleCmd, []string{})
		assert.NoError(t, err)
	})
}
