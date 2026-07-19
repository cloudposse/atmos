package exec

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newVendorPullFlagSet mirrors the flags cmd/vendor/vendor.go registers on the
// `atmos vendor pull` command, so parseVendorFlags tests exercise the exact flag
// shape parseVendorFlags is invoked with in production.
func newVendorPullFlagSet(withRefreshLock bool) *pflag.FlagSet {
	flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	flags.String("component", "", "")
	flags.String("stack", "", "")
	flags.String("tags", "", "")
	flags.Bool("everything", false, "")
	if withRefreshLock {
		flags.Bool("refresh-lock", false, "")
	}
	return flags
}

// TestParseVendorFlags_RefreshLock proves parseVendorFlags reads the `refresh-lock` flag when the
// calling command registers it (cmd/vendor/vendor.go's vendorPullCmd), and defaults to false
// without erroring when the flag isn't registered at all (parseVendorFlags is also reachable from
// paths that don't define it).
func TestParseVendorFlags_RefreshLock(t *testing.T) {
	t.Run("refresh-lock flag set to true is read", func(t *testing.T) {
		flags := newVendorPullFlagSet(true)
		require.NoError(t, flags.Set("refresh-lock", "true"))

		vendorFlags, err := parseVendorFlags(flags)

		require.NoError(t, err)
		assert.True(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag left at its default is false", func(t *testing.T) {
		flags := newVendorPullFlagSet(true)

		vendorFlags, err := parseVendorFlags(flags)

		require.NoError(t, err)
		assert.False(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag not registered at all does not error", func(t *testing.T) {
		flags := newVendorPullFlagSet(false)

		vendorFlags, err := parseVendorFlags(flags)

		require.NoError(t, err)
		assert.False(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag registered with the wrong type surfaces GetBool's error", func(t *testing.T) {
		flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
		flags.Bool("dry-run", false, "")
		flags.String("component", "", "")
		flags.String("stack", "", "")
		flags.String("tags", "", "")
		flags.Bool("everything", false, "")
		flags.String("refresh-lock", "", "") // Wrong type: GetBool must fail.

		_, err := parseVendorFlags(flags)

		require.Error(t, err)
	})
}
