package ansible

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

func TestAnsibleFlags(t *testing.T) {
	registry := AnsibleFlags()

	t.Run("has common flags", func(t *testing.T) {
		// Should have common flags (stack, dry-run).
		assert.True(t, registry.Has("stack"), "should have stack flag")
		assert.True(t, registry.Has("dry-run"), "should have dry-run flag")
	})

	t.Run("has ansible-specific flags", func(t *testing.T) {
		assert.True(t, registry.Has("playbook"), "should have playbook flag")
		assert.True(t, registry.Has("inventory"), "should have inventory flag")
	})

	t.Run("has correct flag count", func(t *testing.T) {
		// Common flags + ansible-specific flags.
		assert.GreaterOrEqual(t, registry.Count(), 4)
	})
}

func TestAnsibleSpecificFlagProperties(t *testing.T) {
	registry := AnsibleFlags()

	t.Run("playbook flag properties", func(t *testing.T) {
		flag := registry.Get("playbook")
		require.NotNil(t, flag)

		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "playbook should be a StringFlag")

		assert.Equal(t, "playbook", strFlag.Name)
		assert.Equal(t, "p", strFlag.Shorthand)
		assert.Equal(t, "", strFlag.Default)
		assert.Contains(t, strFlag.Description, "playbook")
		assert.Equal(t, []string{"ATMOS_ANSIBLE_PLAYBOOK"}, strFlag.EnvVars)
	})

	t.Run("inventory flag properties", func(t *testing.T) {
		flag := registry.Get("inventory")
		require.NotNil(t, flag)

		strFlag, ok := flag.(*flags.StringFlag)
		require.True(t, ok, "inventory should be a StringFlag")

		assert.Equal(t, "inventory", strFlag.Name)
		assert.Equal(t, "i", strFlag.Shorthand)
		assert.Equal(t, "", strFlag.Default)
		assert.Contains(t, strFlag.Description, "inventory")
		assert.Equal(t, []string{"ATMOS_ANSIBLE_INVENTORY"}, strFlag.EnvVars)
	})
}

func TestWithAnsibleFlags(t *testing.T) {
	t.Run("creates parser with all ansible flags", func(t *testing.T) {
		parser := flags.NewStandardParser(WithAnsibleFlags())

		registry := parser.Registry()

		// Should have all ansible flags (common + ansible-specific).
		assert.GreaterOrEqual(t, registry.Count(), 4)
		assert.True(t, registry.Has("stack"))
		assert.True(t, registry.Has("dry-run"))
		assert.True(t, registry.Has("playbook"))
		assert.True(t, registry.Has("inventory"))
	})
}

func TestAnsibleFlagsCobraRegistration(t *testing.T) {
	t.Run("flags are registered on cobra command", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithAnsibleFlags())
		parser.RegisterFlags(cmd)

		// Verify flags are registered on command.
		flagNames := []string{
			"stack",
			"dry-run",
			"playbook",
			"inventory",
		}

		for _, flagName := range flagNames {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "%s flag should be registered on cobra command", flagName)
		}
	})

	t.Run("playbook flag has correct shorthand", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithAnsibleFlags())
		parser.RegisterFlags(cmd)

		flag := cmd.Flags().Lookup("playbook")
		require.NotNil(t, flag)
		assert.Equal(t, "p", flag.Shorthand)
	})

	t.Run("inventory flag has correct shorthand", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		parser := flags.NewStandardParser(WithAnsibleFlags())
		parser.RegisterFlags(cmd)

		flag := cmd.Flags().Lookup("inventory")
		require.NotNil(t, flag)
		assert.Equal(t, "i", flag.Shorthand)
	})
}

func TestAnsibleFlagsViperBinding(t *testing.T) {
	t.Run("ansible flags bind to viper", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()
		parser := flags.NewStandardParser(WithAnsibleFlags())
		parser.RegisterFlags(cmd)
		err := parser.BindToViper(v)
		require.NoError(t, err)

		// Set values via viper and verify they can be retrieved.
		v.Set("playbook", "site.yml")
		v.Set("inventory", "hosts")
		v.Set("stack", "dev-us-east-1")

		assert.Equal(t, "site.yml", v.GetString("playbook"))
		assert.Equal(t, "hosts", v.GetString("inventory"))
		assert.Equal(t, "dev-us-east-1", v.GetString("stack"))
	})
}

func TestAnsibleFlagsEnvironmentVariables(t *testing.T) {
	registry := AnsibleFlags()

	tests := []struct {
		flagName string
		envVar   string
	}{
		{"playbook", "ATMOS_ANSIBLE_PLAYBOOK"},
		{"inventory", "ATMOS_ANSIBLE_INVENTORY"},
	}

	for _, tc := range tests {
		t.Run(tc.flagName+" has correct env var", func(t *testing.T) {
			flag := registry.Get(tc.flagName)
			require.NotNil(t, flag, "%s flag should exist", tc.flagName)

			strFlag, ok := flag.(*flags.StringFlag)
			require.True(t, ok)
			assert.Contains(t, strFlag.EnvVars, tc.envVar)
		})
	}
}

func TestAnsibleParserPersistentFlags(t *testing.T) {
	t.Run("ansibleParser is initialized", func(t *testing.T) {
		require.NotNil(t, ansibleParser, "ansibleParser should be initialized in init()")
	})

	t.Run("persistent flags are registered on ansibleCmd", func(t *testing.T) {
		// Check that persistent flags from ansibleParser are available on subcommands.
		// Persistent flags should be inherited by subcommands.
		stackFlag := ansibleCmd.PersistentFlags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack should be a persistent flag")

		playbookFlag := ansibleCmd.PersistentFlags().Lookup("playbook")
		assert.NotNil(t, playbookFlag, "playbook should be a persistent flag")

		inventoryFlag := ansibleCmd.PersistentFlags().Lookup("inventory")
		assert.NotNil(t, inventoryFlag, "inventory should be a persistent flag")
	})

	t.Run("persistent flags are inherited by playbook subcommand", func(t *testing.T) {
		// When using Cobra's inheritance, persistent flags should be available
		// on subcommands via InheritedFlags().
		stackFlag := playbookCmd.InheritedFlags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack should be inherited by playbook subcommand")
	})
}
