package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestSelectorFlagPresence(t *testing.T) {
	// Test that the parent commands have the persistent selector flag
	t.Run("list command has persistent selector flag", func(t *testing.T) {
		flag := listCmd.PersistentFlags().Lookup("selector")
		assert.NotNil(t, flag, "selector flag missing on list command")
		assert.Equal(t, "l", flag.Shorthand, "selector flag shorthand mismatch on list command")
	})

	t.Run("describe command has persistent selector flag", func(t *testing.T) {
		flag := describeCmd.PersistentFlags().Lookup("selector")
		assert.NotNil(t, flag, "selector flag missing on describe command")
		assert.Equal(t, "l", flag.Shorthand, "selector flag shorthand mismatch on describe command")
	})

	// Test that subcommands inherit the flag by checking if they can access it
	listSubcommands := []struct {
		name    string
		command *cobra.Command
	}{
		{"list_stacks", listStacksCmd},
		{"list_components", listComponentsCmd},
		{"list_values", listValuesCmd},
		{"list_vars", listVarsCmd},
		{"list_settings", listSettingsCmd},
		{"list_metadata", listMetadataCmd},
	}

	describeSubcommands := []struct {
		name    string
		command *cobra.Command
	}{
		{"describe_component", describeComponentCmd},
		{"describe_stacks", describeStacksCmd},
		{"describe_affected", describeAffectedCmd},
	}

	for _, tt := range listSubcommands {
		t.Run(tt.name+" inherits selector flag", func(t *testing.T) {
			// Simulate command execution to populate inherited flags
			tt.command.SetArgs([]string{"--help"})

			// Check if the command can access the selector flag
			flag := tt.command.Flags().Lookup("selector")
			if flag == nil {
				// Try to get it from the parent
				flag = tt.command.Parent().PersistentFlags().Lookup("selector")
			}
			assert.NotNil(t, flag, "selector flag not accessible on %s", tt.name)
		})
	}

	for _, tt := range describeSubcommands {
		t.Run(tt.name+" inherits selector flag", func(t *testing.T) {
			// Simulate command execution to populate inherited flags
			tt.command.SetArgs([]string{"--help"})

			// Check if the command can access the selector flag
			flag := tt.command.Flags().Lookup("selector")
			if flag == nil {
				// Try to get it from the parent
				flag = tt.command.Parent().PersistentFlags().Lookup("selector")
			}
			assert.NotNil(t, flag, "selector flag not accessible on %s", tt.name)
		})
	}
}
