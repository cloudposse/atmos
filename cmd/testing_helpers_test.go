package cmd

import (
	"os"
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ensureIOInitialized initializes the global I/O writer and ui formatter for
// tests that invoke data.Write*/ui.Write* code paths directly, without going
// through RootCmd.Execute() (whose PersistentPreRun normally does this).
// Needed because restoreRootCmdState (registered via NewTestKit's cleanup)
// resets this global state to nil at the end of every test that uses it, so
// a later test that calls a migrated helper directly can otherwise panic.
func ensureIOInitialized(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

// flagSnapshot stores the state of a flag for restoration.
type flagSnapshot struct {
	value   string
	changed bool
}

// cmdStateSnapshot stores the complete state of RootCmd and I/O for restoration.
type cmdStateSnapshot struct {
	args           []string
	osArgs         []string
	flags          map[*cobra.Command]map[string]flagSnapshot
	chdirProcessed bool
	colorProfile   termenv.Profile // Lipgloss color profile
	openDocsURL    func(string) error
	// childCommands maps every command reachable from RootCmd (including RootCmd
	// itself) to its own Commands() list at snapshot time. Covers grandchildren
	// and deeper: a test that adds/removes a subcommand under e.g. "toolchain"
	// (not RootCmd directly) would otherwise leave that nested registration in
	// the shared Cobra tree for later tests to observe.
	childCommands map[*cobra.Command][]*cobra.Command
}

// walkCommandTree calls fn for RootCmd and every command reachable from it
// (recursively, through every level of subcommands). Used to snapshot/restore
// flag state across the whole command tree, not just RootCmd's own flags: a
// real invocation of e.g. "atmos toolchain --help" through RootCmd.ExecuteC()
// parses --help onto toolchain's own FlagSet, and that FlagSet is a
// package-level singleton no different from RootCmd's -- left un-reset, it
// leaks into whichever later test's dispatch reaches the same subcommand. See
// docs/fixes for the incident this closes.
func walkCommandTree(root *cobra.Command, fn func(*cobra.Command)) {
	fn(root)
	for _, c := range root.Commands() {
		walkCommandTree(c, fn)
	}
}

// snapshotRootCmdState captures the current state of RootCmd (and every
// subcommand reachable from it) including all flag values and I/O streams.
// This allows tests to save state at the beginning and restore it in cleanup
// via NewTestKit, preventing test pollution without needing to maintain a
// hardcoded list of flags.
func snapshotRootCmdState() *cmdStateSnapshot {
	snapshot := &cmdStateSnapshot{
		args:           make([]string, len(RootCmd.Flags().Args())),
		osArgs:         make([]string, len(os.Args)),
		flags:          make(map[*cobra.Command]map[string]flagSnapshot),
		chdirProcessed: chdirProcessed,
		colorProfile:   lipgloss.ColorProfile(),
		openDocsURL:    openDocsURL,
		childCommands:  make(map[*cobra.Command][]*cobra.Command),
	}

	// Copy args.
	copy(snapshot.args, RootCmd.Flags().Args())

	// Copy os.Args.
	copy(snapshot.osArgs, os.Args)

	// Snapshot every command's own flags (both local and persistent) and its
	// own child-command list.
	walkCommandTree(RootCmd, func(c *cobra.Command) {
		flags := make(map[string]flagSnapshot)
		snapshotFlags := func(flagSet *pflag.FlagSet) {
			flagSet.VisitAll(func(f *pflag.Flag) {
				flags[f.Name] = flagSnapshot{
					value:   f.Value.String(),
					changed: f.Changed,
				}
			})
		}
		snapshotFlags(c.Flags())
		snapshotFlags(c.PersistentFlags())
		snapshot.flags[c] = flags
		snapshot.childCommands[c] = append([]*cobra.Command(nil), c.Commands()...)
	})

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

// restoreRootCmdState restores RootCmd, Viper, and I/O to a previously captured state.
func restoreRootCmdState(snapshot *cmdStateSnapshot) {
	// Reset global I/O and UI state BEFORE restoring os std streams.
	// This ensures cached I/O contexts are cleared while tests may still have
	// modified stdout/stderr, preventing the next test from inheriting stale stream references.
	iolib.Reset()
	data.Reset()
	ui.Reset()
	homedir.Reset()

	// Re-establish a valid default I/O context immediately after resetting.
	// Many tests in this package call migrated data.Write*/ui.Write* code paths
	// (directly or transitively) without going through RootCmd.Execute()'s
	// PersistentPreRun, which normally does this initialization. Leaving the
	// global state nil between tests means whichever test happens to run next
	// would panic (data.Write*) or silently no-op (ui.Write*) depending on
	// test execution order — restoring a fresh baseline here keeps ambient
	// I/O state valid the way it always is in the real binary, while a test
	// that explicitly needs its own stream capture can still call
	// iolib.Initialize()/data.InitWriter/ui.InitFormatter itself afterward.
	if ioCtx, err := iolib.NewContext(); err == nil {
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	}

	// Restore command args.
	RootCmd.SetArgs(snapshot.args)

	// Restore os.Args.
	os.Args = make([]string, len(snapshot.osArgs))
	copy(os.Args, snapshot.osArgs)

	// Restore chdirProcessed flag.
	chdirProcessed = snapshot.chdirProcessed

	// Remove any command registered anywhere in the tree since the snapshot was
	// taken (e.g. by a test loading real custom commands via InitCliConfig +
	// processCustomCommands, or one that adds/removes a subcommand under a
	// non-RootCmd parent like "toolchain"). Left in place, a later test can
	// collide with or silently observe a command from an unrelated,
	// already-finished test. Done before the flag walk below so that walk
	// visits exactly the commands present in the snapshot.
	restoreCommandChildren(snapshot)

	// Restore every snapshotted command's flags to their captured values.
	restoreFlagsOn := func(c *cobra.Command) {
		flags, ok := snapshot.flags[c]
		if !ok {
			return
		}
		restoreFlags := func(flagSet *pflag.FlagSet) {
			flagSet.VisitAll(func(f *pflag.Flag) {
				snap, ok := flags[f.Name]
				if !ok {
					return
				}
				// StringSlice/StringArray flags need special handling due to append behavior.
				if f.Value.Type() == "stringSlice" || f.Value.Type() == "stringArray" {
					restoreStringSliceFlag(f, snap)
					return
				}
				// For other flag types, direct Set() works fine.
				_ = f.Value.Set(snap.value)
				f.Changed = snap.changed
			})
		}
		restoreFlags(c.Flags())
		restoreFlags(c.PersistentFlags())
	}
	walkCommandTree(RootCmd, restoreFlagsOn)

	// Restore lipgloss color profile and regenerate theme styles.
	// This prevents test pollution from color settings.
	lipgloss.SetColorProfile(snapshot.colorProfile)
	theme.InvalidateStyleCache()

	// Restore package-level test seams.
	openDocsURL = snapshot.openDocsURL
}

// restoreCommandChildren recursively restores every command's own
// child-command list (not just RootCmd's) to what it was at snapshot time.
// Walking down through the snapshot's own recorded tree (rather than the
// live, possibly-mutated one) ensures every level gets visited even if a
// test added a new subtree of commands that doesn't exist in the snapshot
// at all.
func restoreCommandChildren(snapshot *cmdStateSnapshot) {
	var restore func(c *cobra.Command)
	restore = func(c *cobra.Command) {
		original, ok := snapshot.childCommands[c]
		if !ok {
			return
		}
		originalSet := make(map[*cobra.Command]bool, len(original))
		for _, child := range original {
			originalSet[child] = true
		}
		for _, child := range c.Commands() {
			if !originalSet[child] {
				c.RemoveCommand(child)
			}
		}
		// Cobra's RemoveCommand clears the removed command's Parent(); AddCommand
		// sets it back to the new parent. So a snapshot child whose Parent() is no
		// longer c was removed by the test and must be re-added.
		for _, child := range original {
			if child.Parent() != c {
				c.AddCommand(child)
			}
		}
		for _, child := range original {
			restore(child)
		}
	}
	restore(RootCmd)
}
