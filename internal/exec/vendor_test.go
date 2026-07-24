package exec

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

// newVendorPullFlagSet mirrors the flags cmd/vendor/vendor.go registers on the
// `atmos vendor pull` command, so parseVendorFlags tests exercise the exact flag
// shape parseVendorFlags is invoked with in production.
func newVendorPullFlagSet(withRefreshLock bool) *pflag.FlagSet {
	flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	flags.String("component", "", "")
	flags.String("tags", "", "")
	flags.Bool("everything", false, "")
	if withRefreshLock {
		flags.Bool("refresh-lock", false, "")
	}
	return flags
}

// newVendorPullFlagSetWithLockEnforcement mirrors cmd/vendor/vendor.go's vendorPullCmd flag set
// including the "lock-enforcement" flag, for TestParseVendorFlags_LockEnforcement.
func newVendorPullFlagSetWithLockEnforcement(withFlag bool) *pflag.FlagSet {
	flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	flags.String("component", "", "")
	flags.String("tags", "", "")
	flags.Bool("everything", false, "")
	if withFlag {
		flags.String("lock-enforcement", "", "")
	}
	return flags
}

// TestParseVendorFlags_LockEnforcement proves the --lock-enforcement precedence: an explicitly
// passed flag wins, otherwise a non-empty atmosConfig.Vendor.Lock.Enforcement wins, otherwise
// DefaultLockEnforcement's "warn" fallback applies. Every case matches the precedence
// DefaultLockEnforcement/parseVendorFlags document.
func TestParseVendorFlags_LockEnforcement(t *testing.T) {
	t.Run("explicit flag wins over config", func(t *testing.T) {
		flags := newVendorPullFlagSetWithLockEnforcement(true)
		require.NoError(t, flags.Set("lock-enforcement", "strict"))
		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Vendor.Lock.Enforcement = install.LockEnforcementSilent

		vendorFlags, err := parseVendorFlags(flags, atmosConfig)

		require.NoError(t, err)
		assert.Equal(t, install.LockEnforcementStrict, vendorFlags.LockEnforcement)
	})

	t.Run("config value is used when the flag is not explicitly passed", func(t *testing.T) {
		flags := newVendorPullFlagSetWithLockEnforcement(true)
		atmosConfig := &schema.AtmosConfiguration{}
		atmosConfig.Vendor.Lock.Enforcement = install.LockEnforcementSilent

		vendorFlags, err := parseVendorFlags(flags, atmosConfig)

		require.NoError(t, err)
		assert.Equal(t, install.LockEnforcementSilent, vendorFlags.LockEnforcement)
	})

	t.Run("defaults to warn when neither flag nor config is set", func(t *testing.T) {
		flags := newVendorPullFlagSetWithLockEnforcement(true)

		vendorFlags, err := parseVendorFlags(flags, &schema.AtmosConfiguration{})

		require.NoError(t, err)
		assert.Equal(t, install.LockEnforcementWarn, vendorFlags.LockEnforcement)
	})

	t.Run("defaults to warn when atmosConfig is nil", func(t *testing.T) {
		flags := newVendorPullFlagSetWithLockEnforcement(true)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, install.LockEnforcementWarn, vendorFlags.LockEnforcement)
	})

	t.Run("flag not registered at all does not error and still resolves the config/default value", func(t *testing.T) {
		flags := newVendorPullFlagSetWithLockEnforcement(false)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, install.LockEnforcementWarn, vendorFlags.LockEnforcement)
	})
}

// TestValidateVendorFlags_LockEnforcement proves validateVendorFlags accepts every documented
// enforcement level and rejects anything else.
func TestValidateVendorFlags_LockEnforcement(t *testing.T) {
	for _, value := range []string{install.LockEnforcementStrict, install.LockEnforcementWarn, install.LockEnforcementSilent, ""} {
		t.Run("accepts "+value, func(t *testing.T) {
			require.NoError(t, validateVendorFlags(&VendorFlags{LockEnforcement: value}))
		})
	}

	t.Run("rejects an unrecognized value", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{LockEnforcement: "bogus"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidLockEnforcement)
	})
}

// TestParseVendorFlags_RefreshLock proves parseVendorFlags reads the `refresh-lock` flag when the
// calling command registers it (cmd/vendor/vendor.go's vendorPullCmd), and defaults to false
// without erroring when the flag isn't registered at all (parseVendorFlags is also reachable from
// paths that don't define it).
func TestParseVendorFlags_RefreshLock(t *testing.T) {
	t.Run("refresh-lock flag set to true is read", func(t *testing.T) {
		flags := newVendorPullFlagSet(true)
		require.NoError(t, flags.Set("refresh-lock", "true"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.True(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag left at its default is false", func(t *testing.T) {
		flags := newVendorPullFlagSet(true)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.False(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag not registered at all does not error", func(t *testing.T) {
		flags := newVendorPullFlagSet(false)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.False(t, vendorFlags.RefreshLock)
	})

	t.Run("refresh-lock flag registered with the wrong type surfaces GetBool's error", func(t *testing.T) {
		flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
		flags.Bool("dry-run", false, "")
		flags.String("component", "", "")
		flags.String("tags", "", "")
		flags.Bool("everything", false, "")
		flags.String("refresh-lock", "", "") // Wrong type: GetBool must fail.

		_, err := parseVendorFlags(flags, nil)

		require.Error(t, err)
	})
}

// newVendorPullFlagSetWithType mirrors cmd/vendor/vendor.go's vendorPullCmd flag set including the
// "type" flag (default "terraform", matching production), for TestParseVendorFlags_TypeChanged.
func newVendorPullFlagSetWithType(withType bool) *pflag.FlagSet {
	flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	flags.String("component", "", "")
	flags.String("tags", "", "")
	flags.Bool("everything", false, "")
	if withType {
		flags.StringP("type", "t", "terraform", "")
	}
	return flags
}

// TestParseVendorFlags_TypeChanged proves parseVendorFlags tracks whether --type was explicitly
// passed (flags.Changed("type")), not just its resolved value - handleVendorPullSweep needs this to
// distinguish "the user wants only this one type" from "no --type given, sweep every type", since
// --type defaults to a non-empty "terraform" on vendorPullCmd (cmd/vendor/vendor.go).
func TestParseVendorFlags_TypeChanged(t *testing.T) {
	t.Run("type flag left at its default is not changed", func(t *testing.T) {
		flags := newVendorPullFlagSetWithType(true)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "terraform", vendorFlags.ComponentType)
		assert.False(t, vendorFlags.TypeChanged)
	})

	t.Run("type flag explicitly set to its default value is still changed", func(t *testing.T) {
		flags := newVendorPullFlagSetWithType(true)
		require.NoError(t, flags.Set("type", "terraform"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "terraform", vendorFlags.ComponentType)
		assert.True(t, vendorFlags.TypeChanged, "explicitly setting --type must be tracked even when the value equals the default")
	})

	t.Run("type flag explicitly set to a non-default value is changed", func(t *testing.T) {
		flags := newVendorPullFlagSetWithType(true)
		require.NoError(t, flags.Set("type", "helmfile"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "helmfile", vendorFlags.ComponentType)
		assert.True(t, vendorFlags.TypeChanged)
	})

	t.Run("type flag not registered at all does not error and is not changed", func(t *testing.T) {
		flags := newVendorPullFlagSetWithType(false)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.False(t, vendorFlags.TypeChanged)
	})
}
