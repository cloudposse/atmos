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

// newVendorPullFlagSetWithStack mirrors cmd/vendor/vendor.go's vendorPullCmd flag set including
// the "stack" flag, for TestParseVendorFlags_Stack.
func newVendorPullFlagSetWithStack(withStack bool) *pflag.FlagSet {
	flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
	flags.Bool("dry-run", false, "")
	flags.String("component", "", "")
	flags.String("tags", "", "")
	flags.Bool("everything", false, "")
	if withStack {
		flags.StringP("stack", "s", "", "")
	}
	return flags
}

// TestParseVendorFlags_Stack proves parseVendorFlags reads --stack when the calling command
// registers it (cmd/vendor/vendor.go's vendorPullCmd), and defaults to "" without erroring when
// the flag isn't registered at all ('vendor update --pull' delegates to parseVendorFlags with a
// FlagSet that doesn't define "stack").
func TestParseVendorFlags_Stack(t *testing.T) {
	t.Run("stack flag set is read", func(t *testing.T) {
		flags := newVendorPullFlagSetWithStack(true)
		require.NoError(t, flags.Set("stack", "dev-us-west-2"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "dev-us-west-2", vendorFlags.Stack)
	})

	t.Run("stack flag left at its default is empty", func(t *testing.T) {
		flags := newVendorPullFlagSetWithStack(true)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Empty(t, vendorFlags.Stack)
	})

	t.Run("stack flag not registered at all does not error", func(t *testing.T) {
		flags := newVendorPullFlagSetWithStack(false)

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Empty(t, vendorFlags.Stack)
	})
}

// TestValidateVendorFlags_Stack proves validateVendorFlags rejects --stack combined with
// --component or --everything, matching the existing --tags mutual-exclusivity rules, while
// allowing --stack on its own (including together with --tags, which parseVendorFlags never
// threads through the stack-based handleStackVendor path but validateVendorFlags does not reject).
func TestValidateVendorFlags_Stack(t *testing.T) {
	t.Run("stack alone is valid", func(t *testing.T) {
		require.NoError(t, validateVendorFlags(&VendorFlags{Stack: "dev-us-west-2"}))
	})

	t.Run("component and stack together is rejected", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{Component: "vpc", Stack: "dev-us-west-2"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrValidateComponentStackFlag)
	})

	t.Run("everything and stack together is rejected", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{Everything: true, Stack: "dev-us-west-2"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrValidateEverythingFlag)
	})
}

// TestValidateVendorFlags_Labels proves validateVendorFlags rejects --labels combined with
// --component, --tags, or --everything (mirroring --stack's own exclusivity rules), while allowing
// --labels on its own and together with --stack (both resolve the same stack-declared component set
// -- --labels narrows it further, not a separate mode).
func TestValidateVendorFlags_Labels(t *testing.T) {
	t.Run("labels alone is valid", func(t *testing.T) {
		require.NoError(t, validateVendorFlags(&VendorFlags{Labels: map[string]string{"tier": "1"}}))
	})

	t.Run("stack and labels together is valid", func(t *testing.T) {
		require.NoError(t, validateVendorFlags(&VendorFlags{Stack: "dev-us-west-2", Labels: map[string]string{"tier": "1"}}))
	})

	t.Run("component and labels together is rejected", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{Component: "vpc", Labels: map[string]string{"tier": "1"}})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrValidateComponentLabelsFlag)
	})

	t.Run("tags and labels together is rejected", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{Tags: []string{"networking"}, Labels: map[string]string{"tier": "1"}})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrValidateTagsLabelsFlag)
	})

	t.Run("everything and labels together is rejected", func(t *testing.T) {
		err := validateVendorFlags(&VendorFlags{Everything: true, Labels: map[string]string{"tier": "1"}})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrValidateEverythingFlag)
	})
}

// TestParseOptionalLabelsFlag proves parseOptionalLabelsFlag reads --labels when the calling
// command registers it, defaults to nil without erroring when the flag isn't registered at all
// ('vendor update --pull' delegates to parseVendorFlags with a FlagSet that doesn't define
// "labels"), and propagates a malformed value's parse error.
func TestParseOptionalLabelsFlag(t *testing.T) {
	t.Run("labels flag set is read", func(t *testing.T) {
		flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
		flags.String("labels", "", "")
		require.NoError(t, flags.Set("labels", "tier=1,cost-center:platform"))

		labels, err := parseOptionalLabelsFlag(flags)

		require.NoError(t, err)
		assert.Equal(t, map[string]string{"tier": "1", "cost-center": "platform"}, labels)
	})

	t.Run("labels flag not registered at all does not error", func(t *testing.T) {
		flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)

		labels, err := parseOptionalLabelsFlag(flags)

		require.NoError(t, err)
		assert.Nil(t, labels)
	})

	t.Run("malformed labels value propagates a parse error", func(t *testing.T) {
		flags := pflag.NewFlagSet("vendor pull", pflag.ContinueOnError)
		flags.String("labels", "", "")
		require.NoError(t, flags.Set("labels", "not-a-pair"))

		_, err := parseOptionalLabelsFlag(flags)

		require.Error(t, err)
	})
}

// TestSetDefaultEverythingFlag_Stack proves --stack alone (like --component and --tags) suppresses
// the "no flags given" default that otherwise sets Everything to true.
func TestSetDefaultEverythingFlag_Stack(t *testing.T) {
	t.Run("stack set suppresses the everything default", func(t *testing.T) {
		flags := newVendorPullFlagSetWithStack(true)
		vendorFlags := &VendorFlags{Stack: "dev-us-west-2"}

		setDefaultEverythingFlag(flags, vendorFlags)

		assert.False(t, vendorFlags.Everything)
	})

	t.Run("no flags given still defaults everything to true", func(t *testing.T) {
		flags := newVendorPullFlagSetWithStack(true)
		vendorFlags := &VendorFlags{}

		setDefaultEverythingFlag(flags, vendorFlags)

		assert.True(t, vendorFlags.Everything)
	})
}

// TestParseVendorFlags_ComponentSliceFlag proves parseVendorFlags tolerates the flag shape
// `vendor update --pull` delegates with: vendorUpdateCmd registers --component as a repeatable
// string slice (cmd/vendor/update.go), while vendorPullCmd registers a plain string. A
// stringSlice-typed --component must parse (not hard-error via pflag's GetString type check),
// yield the single element when one is set, and reject multi-element selections explicitly.
func TestParseVendorFlags_ComponentSliceFlag(t *testing.T) {
	newSliceFlagSet := func() *pflag.FlagSet {
		flags := pflag.NewFlagSet("vendor update", pflag.ContinueOnError)
		flags.Bool("dry-run", false, "")
		flags.StringSlice("component", []string{}, "")
		flags.String("tags", "", "")
		flags.Bool("everything", false, "")
		return flags
	}

	t.Run("unset slice flag parses as empty component", func(t *testing.T) {
		vendorFlags, err := parseVendorFlags(newSliceFlagSet(), nil)

		require.NoError(t, err)
		assert.Empty(t, vendorFlags.Component)
	})

	t.Run("single-element slice yields that component", func(t *testing.T) {
		flags := newSliceFlagSet()
		require.NoError(t, flags.Set("component", "vpc"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "vpc", vendorFlags.Component)
	})

	t.Run("multi-element slice is rejected with the sentinel", func(t *testing.T) {
		flags := newSliceFlagSet()
		require.NoError(t, flags.Set("component", "vpc"))
		require.NoError(t, flags.Set("component", "eks"))

		_, err := parseVendorFlags(flags, nil)

		require.ErrorIs(t, err, ErrSingleComponentRequired)
	})

	t.Run("string-typed component flag still parses", func(t *testing.T) {
		flags := newVendorPullFlagSet(false)
		require.NoError(t, flags.Set("component", "vpc"))

		vendorFlags, err := parseVendorFlags(flags, nil)

		require.NoError(t, err)
		assert.Equal(t, "vpc", vendorFlags.Component)
	})
}
