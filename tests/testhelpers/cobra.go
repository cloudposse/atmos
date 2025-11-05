package testhelpers

import (
	"os"
	"reflect"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
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

// CobraStateSnapshot stores the complete state of a cobra.Command for restoration.
// This is used by TestKit to prevent test pollution from global command state.
type CobraStateSnapshot struct {
	args   []string
	osArgs []string
	flags  map[string]FlagSnapshot
}

// FlagSnapshot stores the state of a single flag for restoration.
type FlagSnapshot struct {
	Value   string
	Changed bool
}

// SnapshotCobraState captures the current state of a cobra.Command including all flag values.
// This is a generic helper that works with any cobra.Command, not just RootCmd.
func SnapshotCobraState(cmd *cobra.Command) *CobraStateSnapshot {
	snapshot := &CobraStateSnapshot{
		args:   make([]string, len(cmd.Flags().Args())),
		osArgs: make([]string, len(os.Args)),
		flags:  make(map[string]FlagSnapshot),
	}

	// Copy args.
	copy(snapshot.args, cmd.Flags().Args())

	// Copy os.Args.
	copy(snapshot.osArgs, os.Args)

	// Snapshot all flags (both local and persistent).
	snapshotFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			snapshot.flags[f.Name] = FlagSnapshot{
				Value:   f.Value.String(),
				Changed: f.Changed,
			}
		})
	}

	snapshotFlags(cmd.Flags())
	snapshotFlags(cmd.PersistentFlags())

	return snapshot
}

// restoreStringSliceFlag handles restoration of StringSlice/StringArray flags.
// These flag types have Set() methods that append rather than replace, so we need
// to use reflection to clear the underlying slice first.
func restoreStringSliceFlag(f *pflag.Flag, snap FlagSnapshot) {
	// Use reflection to access the underlying slice and clear it.
	v := reflect.ValueOf(f.Value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	// Look for a field that holds the slice (usually "value").
	if v.Kind() == reflect.Struct {
		valueField := v.FieldByName("value")
		if valueField.IsValid() && valueField.CanSet() {
			// Reset to empty slice to prevent append behavior.
			valueField.Set(reflect.MakeSlice(valueField.Type(), 0, 0))
		}
	}
	// Reset Changed state before setting value.
	f.Changed = false
	// Set the snapshot value if not default.
	if snap.Value != "[]" && snap.Value != "" {
		_ = f.Value.Set(snap.Value)
	}
	// Restore Changed state.
	f.Changed = snap.Changed
}

// RestoreCobraState restores a cobra.Command to a previously captured state.
// This is a generic helper that works with any cobra.Command, not just RootCmd.
func RestoreCobraState(cmd *cobra.Command, snapshot *CobraStateSnapshot) {
	// Restore command args.
	cmd.SetArgs(snapshot.args)

	// Restore os.Args.
	os.Args = make([]string, len(snapshot.osArgs))
	copy(os.Args, snapshot.osArgs)

	// Restore all flags to their snapshotted values.
	restoreFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot.flags[f.Name]; ok {
				// StringSlice/StringArray flags need special handling due to append behavior.
				if f.Value.Type() == "stringSlice" || f.Value.Type() == "stringArray" {
					restoreStringSliceFlag(f, snap)
					return
				}
				// For other flag types, direct Set() works fine.
				_ = f.Value.Set(snap.Value)
				f.Changed = snap.Changed
			}
		})
	}

	restoreFlags(cmd.Flags())
	restoreFlags(cmd.PersistentFlags())

	// Restore Viper state to prevent test pollution.
	// Problem: Viper is a global singleton. When a test sets format=dotenv,
	// it persists and pollutes subsequent tests expecting the default.
	//
	// Solution: For flags that changed during test but should be unChanged after restore,
	// explicitly set their Viper value back to the flag's default.
	// This lets Viper return the correct default value without breaking SetDefault().
	v := viper.GetViper()

	resetViperToDefault := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			if snap, ok := snapshot.flags[f.Name]; ok {
				// If flag should be unChanged after restore, reset Viper to flag default.
				if !snap.Changed && !f.Changed {
					// Get the default from the restored flag value.
					// After restoreFlags() above, f.Value contains the snapshot default.
					v.Set(f.Name, f.Value.String())
				}
			}
		})
	}

	resetViperToDefault(cmd.Flags())
	resetViperToDefault(cmd.PersistentFlags())
}
