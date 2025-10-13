package testhelpers

import (
	"github.com/spf13/cobra"
)

// NewMockCommand creates a mock Cobra command with the specified flags for testing.
// This helper simplifies test setup by providing a fluent interface for creating commands.
//
// Example:
//
//	cmd := NewMockCommand("test", map[string]string{
//	    "stack": "dev",
//	    "format": "yaml",
//	})
func NewMockCommand(name string, flags map[string]string) *cobra.Command {
	cmd := &cobra.Command{Use: name}

	for flagName, flagValue := range flags {
		cmd.Flags().String(flagName, "", "")
		_ = cmd.Flags().Set(flagName, flagValue)
	}

	return cmd
}

// NewMockCommandWithBool creates a mock Cobra command with boolean flags.
func NewMockCommandWithBool(name string, boolFlags map[string]bool) *cobra.Command {
	cmd := &cobra.Command{Use: name}

	for flagName, flagValue := range boolFlags {
		cmd.Flags().Bool(flagName, false, "")
		if flagValue {
			_ = cmd.Flags().Set(flagName, "true")
		}
	}

	return cmd
}

// NewMockCommandWithMixed creates a mock Cobra command with both string and boolean flags.
func NewMockCommandWithMixed(name string, stringFlags map[string]string, boolFlags map[string]bool) *cobra.Command {
	cmd := &cobra.Command{Use: name}

	for flagName, flagValue := range stringFlags {
		cmd.Flags().String(flagName, "", "")
		_ = cmd.Flags().Set(flagName, flagValue)
	}

	for flagName, flagValue := range boolFlags {
		cmd.Flags().Bool(flagName, false, "")
		if flagValue {
			_ = cmd.Flags().Set(flagName, "true")
		}
	}

	return cmd
}
