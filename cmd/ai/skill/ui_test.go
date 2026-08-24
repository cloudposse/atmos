package skill

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func setupSkillCommandUI(t *testing.T) *bytes.Buffer {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(testStreams{
		input:  bytes.NewBuffer(nil),
		output: &stdout,
		error:  &stderr,
	}))
	require.NoError(t, err)

	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)

	return &stderr
}

// resetStringSliceFlagForTest fully resets the --client repeatable flag
// between tests. The installCmd/uninstallCmd commands are package-level
// singletons initialized once via init(), so pflag's stringSliceValue permanently switches
// from "replace" to "append" mode the first time the flag is ever Set() in this
// process -- afterward, Set("") is a silent no-op rather than a reset. Replace()
// bypasses that append logic and always fully overwrites the value, so combined
// with flag.Changed = false this is a real reset regardless of what any other
// test in this package already did to the flag.
func resetStringSliceFlagForTest(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	flag := cmd.Flags().Lookup(clientFlag)
	require.NotNil(t, flag, "%s flag must be registered", clientFlag)

	sliceValue, ok := flag.Value.(pflag.SliceValue)
	require.True(t, ok, "%s must be a StringSliceFlag", clientFlag)
	require.NoError(t, sliceValue.Replace(nil))

	flag.Changed = false
}

// resetFlagChangedForTest resets a scalar (string/bool) flag's value to its
// default and clears Changed. The installCmd/uninstallCmd commands are
// package-level singletons initialized once via init(), so once any test in this package
// explicitly Set()s a flag (e.g. --scope), Changed stays permanently true for
// the rest of the test binary -- a later test asserting "this flag was never
// touched" behavior (e.g. the --path-without-distribution-flags warning check)
// needs this to start from a known-clean state regardless of what ran before it.
func resetFlagChangedForTest(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()

	flag := cmd.Flags().Lookup(name)
	require.NotNil(t, flag, "%s flag must be registered", name)
	require.NoError(t, flag.Value.Set(flag.DefValue))
	flag.Changed = false
}
