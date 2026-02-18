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

func TestRegisterAnsibleCompletions(t *testing.T) {
	t.Run("registers completions on playbook subcommand", func(t *testing.T) {
		// Create a test command structure.
		testCmd := &cobra.Command{Use: "ansible"}
		playbookSubCmd := &cobra.Command{Use: "playbook"}
		versionSubCmd := &cobra.Command{Use: "version"}
		testCmd.AddCommand(playbookSubCmd)
		testCmd.AddCommand(versionSubCmd)

		// Before registration, ValidArgsFunction should be nil.
		assert.Nil(t, playbookSubCmd.ValidArgsFunction)
		assert.Nil(t, versionSubCmd.ValidArgsFunction)

		// Register completions.
		RegisterAnsibleCompletions(testCmd)

		// After registration, playbook should have a completion function.
		assert.NotNil(t, playbookSubCmd.ValidArgsFunction)
		// Version should still be nil (doesn't need component completion).
		assert.Nil(t, versionSubCmd.ValidArgsFunction)
	})

	t.Run("handles command with no subcommands", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "ansible"}
		// Should not panic.
		RegisterAnsibleCompletions(testCmd)
	})
}

func TestComponentArgCompletion(t *testing.T) {
	t.Run("returns no completions when component already provided", func(t *testing.T) {
		cmd := &cobra.Command{Use: "playbook"}
		args := []string{"existing-component"}

		completions, directive := componentArgCompletion(cmd, args, "")

		assert.Nil(t, completions)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

func TestPlaybookCmdArgsValidation(t *testing.T) {
	t.Run("requires exactly one argument", func(t *testing.T) {
		assert.NotNil(t, playbookCmd.Args)
		// Test with no arguments.
		err := playbookCmd.Args(playbookCmd, []string{})
		assert.Error(t, err)

		// Test with one argument.
		err = playbookCmd.Args(playbookCmd, []string{"component"})
		assert.NoError(t, err)

		// Test with two arguments.
		err = playbookCmd.Args(playbookCmd, []string{"comp1", "comp2"})
		assert.Error(t, err)
	})
}

func TestProcessArgsEdgeCases(t *testing.T) {
	t.Run("handles args with double dash separator", func(t *testing.T) {
		args := []string{"my-component", "--", "--verbose"}
		component, additionalArgs := processArgs(args)
		assert.Equal(t, "my-component", component)
		assert.Equal(t, []string{"--", "--verbose"}, additionalArgs)
	})

	t.Run("handles args with equals sign", func(t *testing.T) {
		args := []string{"my-component", "--limit=webservers"}
		component, additionalArgs := processArgs(args)
		assert.Equal(t, "my-component", component)
		assert.Equal(t, []string{"--limit=webservers"}, additionalArgs)
	})

	t.Run("handles args with special characters", func(t *testing.T) {
		args := []string{"my-component", "-e", "var=value with spaces"}
		component, additionalArgs := processArgs(args)
		assert.Equal(t, "my-component", component)
		assert.Equal(t, []string{"-e", "var=value with spaces"}, additionalArgs)
	})
}

func TestBuildConfigAndStacksInfoEdgeCases(t *testing.T) {
	t.Run("handles command with no flags defined", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		// No flags defined, should return empty info without panic.
		info := buildConfigAndStacksInfo(cmd)
		assert.Equal(t, schema.ConfigAndStacksInfo{}, info)
	})

	t.Run("handles stack flag with empty value", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("stack", "", "stack name")
		// Flag is defined but not set - should return empty stack.
		info := buildConfigAndStacksInfo(cmd)
		assert.Empty(t, info.Stack)
	})
}

func TestInitConfigAndStacksInfoEdgeCases(t *testing.T) {
	t.Run("sets component type correctly for different subcommands", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}

		// Test playbook subcommand.
		info := initConfigAndStacksInfo(cmd, "playbook", []string{})
		assert.Equal(t, "ansible", info.ComponentType)
		assert.Equal(t, "playbook", info.SubCommand)
		assert.Equal(t, []string{"ansible", "playbook"}, info.CliArgs)

		// Test version subcommand.
		info = initConfigAndStacksInfo(cmd, "version", []string{})
		assert.Equal(t, "ansible", info.ComponentType)
		assert.Equal(t, "version", info.SubCommand)
		assert.Equal(t, []string{"ansible", "version"}, info.CliArgs)
	})

	t.Run("preserves all additional args and flags", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		args := []string{"webserver", "-v", "--check", "--diff", "-e", "env=prod"}

		info := initConfigAndStacksInfo(cmd, "playbook", args)

		assert.Equal(t, "webserver", info.ComponentFromArg)
		assert.Equal(t, []string{"-v", "--check", "--diff", "-e", "env=prod"}, info.AdditionalArgsAndFlags)
	})
}

func TestGetAnsibleFlagsEdgeCases(t *testing.T) {
	t.Run("handles flags with whitespace values", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("playbook", "", "playbook")
		cmd.Flags().String("inventory", "", "inventory")
		err := cmd.Flags().Set("playbook", "  site.yml  ")
		require.NoError(t, err)

		flags := getAnsibleFlags(cmd)
		// Note: flag values are not trimmed by default.
		assert.Equal(t, "  site.yml  ", flags.Playbook)
	})

	t.Run("handles flags with path values", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("playbook", "", "playbook")
		cmd.Flags().String("inventory", "", "inventory")
		err := cmd.Flags().Set("playbook", "/opt/playbooks/deploy.yml")
		require.NoError(t, err)
		err = cmd.Flags().Set("inventory", "/etc/ansible/hosts")
		require.NoError(t, err)

		flags := getAnsibleFlags(cmd)
		assert.Equal(t, "/opt/playbooks/deploy.yml", flags.Playbook)
		assert.Equal(t, "/etc/ansible/hosts", flags.Inventory)
	})
}
