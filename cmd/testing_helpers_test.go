package cmd

import (
	"os"
	"reflect"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// flagSnapshot stores the state of a flag for restoration.
type flagSnapshot struct {
	value   string
	changed bool
}

// cmdStateSnapshot stores the complete state of RootCmd for restoration.
type cmdStateSnapshot struct {
	args           []string
	osArgs         []string
	flags          map[string]flagSnapshot
	chdirProcessed bool
	colorProfile   termenv.Profile // Lipgloss color profile
}

// snapshotRootCmdState captures the current state of RootCmd including all flag values.
// This allows tests to save state at the beginning and restore it in cleanup via NewTestKit,
// preventing test pollution without needing to maintain a hardcoded list of flags.
func snapshotRootCmdState() *cmdStateSnapshot {
	snapshot := &cmdStateSnapshot{
		args:           make([]string, len(RootCmd.Flags().Args())),
		osArgs:         make([]string, len(os.Args)),
		flags:          make(map[string]flagSnapshot),
		chdirProcessed: chdirProcessed,
		colorProfile:   lipgloss.ColorProfile(),
	}

	// Copy args.
	copy(snapshot.args, RootCmd.Flags().Args())

	// Copy os.Args.
	copy(snapshot.osArgs, os.Args)

	// Snapshot all flags (both local and persistent).
	snapshotFlags := func(flagSet *pflag.FlagSet) {
		flagSet.VisitAll(func(f *pflag.Flag) {
			snapshot.flags[f.Name] = flagSnapshot{
				value:   f.Value.String(),
				changed: f.Changed,
			}
		})
	}

	snapshotFlags(RootCmd.Flags())
	snapshotFlags(RootCmd.PersistentFlags())

	return snapshot
}

// restoreStringSliceFlag handles restoration of StringSlice/StringArray flags.
// These flag types have Set() methods that append rather than replace, so we need
// to use reflection to clear the underlying slice first.
func restoreStringSliceFlag(f *pflag.Flag, snap flagSnapshot) {
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
	if snap.value != "[]" && snap.value != "" {
		_ = f.Value.Set(snap.value)
	}
	// Restore Changed state.
	f.Changed = snap.changed
}

// restoreRootCmdState restores RootCmd to a previously captured state.
func restoreRootCmdState(snapshot *cmdStateSnapshot) {
	// Restore command args.
	RootCmd.SetArgs(snapshot.args)

	// Restore os.Args.
	os.Args = make([]string, len(snapshot.osArgs))
	copy(os.Args, snapshot.osArgs)

	// Restore chdirProcessed flag.
	chdirProcessed = snapshot.chdirProcessed

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
				_ = f.Value.Set(snap.value)
				f.Changed = snap.changed
			}
		})
	}

	restoreFlags(RootCmd.Flags())
	restoreFlags(RootCmd.PersistentFlags())

	// Restore lipgloss color profile and regenerate theme styles.
	// This prevents test pollution from color settings.
	lipgloss.SetColorProfile(snapshot.colorProfile)
	theme.InvalidateStyleCache()
}
