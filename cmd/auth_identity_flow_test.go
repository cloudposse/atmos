package cmd

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// newIdentityFlowTestCmd returns a *cobra.Command with the --identity flag
// registered (and NoOptDefVal wired the same way cmd/auth.go wires it in
// production), enough for identityFromFlagOrDefault to exercise
// GetIdentityFromFlags.
//
// Most cases drive the helper through Cobra (cmd.Flags().Set) and viper —
// that avoids racing other tests on os.Args. The exception is the no-value
// `--identity` path: NoOptDefVal only fires through real flag parsing, so
// those subtests must call cmd.ParseFlags + withIdentityFlowArgs to mirror
// the production flow (GetIdentityFromFlags consults os.Args to work around
// Cobra's NoOptDefVal quirk).
func newIdentityFlowTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP(IdentityFlagName, "i", "", "")
	// Wire NoOptDefVal the same way cmd/auth.go does for the real
	// persistent flag. Without this, a test that exercises the real
	// no-value `--identity` flow can't distinguish between "production
	// still wires NoOptDefVal" and "we hardcoded the sentinel".
	cmd.Flags().Lookup(IdentityFlagName).NoOptDefVal = IdentityFlagSelectValue
	return cmd
}

// withIdentityFlowArgs swaps os.Args for the duration of a single test.
// The helper identityFromFlagOrDefault passes os.Args to
// GetIdentityFromFlags; the no-value `--identity` path requires the
// flag to appear in os.Args for GetIdentityFromFlags to produce
// IdentityFlagSelectValue through its os.Args-parsing workaround.
// Restores on cleanup.
func withIdentityFlowArgs(t *testing.T, args ...string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

// resetIdentityViper clears any lingering identity flag state in the global
// viper singleton and restores the original value on test completion.
//
// Important: callers must NOT use t.Parallel(). This helper mutates the
// process-wide viper.GetViper() instance and relies on a t.Cleanup restore
// that runs when the calling subtest ends. Parallel subtests (here or in
// another file in the cmd package that touches IdentityFlagName) would
// race each other against that single viper.
func resetIdentityViper(t *testing.T) {
	t.Helper()
	v := viper.GetViper()
	orig := v.Get(IdentityFlagName)
	v.Set(IdentityFlagName, "")
	t.Cleanup(func() { v.Set(IdentityFlagName, orig) })
}

// TestIdentityFromFlagOrDefault covers the shared helper used by every auth
// subcommand (env, console, login, shell, whoami, exec) to resolve the
// effective identity from --identity or the manager's default.
//
// This helper is the source of the wrapped ErrNoDefaultIdentity that the
// fallback dispatcher (maybeOfferProfileFallbackOnAuthConfigError) keys off,
// so its error-wrapping contract is load-bearing across all auth commands.
func TestIdentityFromFlagOrDefault(t *testing.T) {
	t.Run("flag set to explicit identity returns it without calling GetDefaultIdentity", func(t *testing.T) {
		_ = NewTestKit(t)
		resetIdentityViper(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// GetDefaultIdentity must NOT be called when identity flag is set.
		m := types.NewMockAuthManager(ctrl)

		cmd := newIdentityFlowTestCmd(t)
		require.NoError(t, cmd.Flags().Set(IdentityFlagName, "my-identity"))

		got, err := identityFromFlagOrDefault(cmd, m)
		require.NoError(t, err)
		assert.Equal(t, "my-identity", got)
	})

	t.Run("flag unset calls GetDefaultIdentity(false) and returns result", func(t *testing.T) {
		_ = NewTestKit(t)
		resetIdentityViper(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := types.NewMockAuthManager(ctrl)
		m.EXPECT().GetDefaultIdentity(false).Return("default-id", nil)

		cmd := newIdentityFlowTestCmd(t)

		got, err := identityFromFlagOrDefault(cmd, m)
		require.NoError(t, err)
		assert.Equal(t, "default-id", got)
	})

	t.Run("--identity without value (NoOptDefVal) calls GetDefaultIdentity(true) for interactive selection", func(t *testing.T) {
		// Exercises the REAL Cobra NoOptDefVal wiring rather than
		// directly injecting the sentinel. A regression that drops
		// `identityFlag.NoOptDefVal = IdentityFlagSelectValue` from
		// cmd/auth.go would make this test fail, because without that
		// wiring, `--identity` without a value would either error out
		// (default behavior) or set the flag to "" (no NoOptDefVal),
		// neither of which routes to forceSelect=true.
		_ = NewTestKit(t)
		resetIdentityViper(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		m := types.NewMockAuthManager(ctrl)
		// The key contract: forceSelect=true is passed through verbatim.
		m.EXPECT().GetDefaultIdentity(true).Return("picked-id", nil)

		// os.Args must also contain `--identity` (no value) because
		// GetIdentityFromFlags consults it to work around Cobra's
		// NoOptDefVal quirk. See extractIdentityFromArgs().
		withIdentityFlowArgs(t, "atmos", "auth", "whoami", "--identity")

		cmd := newIdentityFlowTestCmd(t)
		// ParseFlags fires NoOptDefVal → flag value becomes
		// IdentityFlagSelectValue, matching production's real parse path.
		require.NoError(t, cmd.ParseFlags([]string{"--identity"}))

		got, err := identityFromFlagOrDefault(cmd, m)
		require.NoError(t, err)
		assert.Equal(t, "picked-id", got)
	})

	t.Run("GetDefaultIdentity error is wrapped with ErrNoDefaultIdentity sentinel", func(t *testing.T) {
		_ = NewTestKit(t)
		resetIdentityViper(t)

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		underlying := errors.New("no identities in config")
		m := types.NewMockAuthManager(ctrl)
		m.EXPECT().GetDefaultIdentity(false).Return("", underlying)

		cmd := newIdentityFlowTestCmd(t)

		got, err := identityFromFlagOrDefault(cmd, m)
		require.Error(t, err)
		assert.Empty(t, got)
		// The sentinel must survive wrapping so the fallback dispatcher can detect it.
		assert.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity,
			"wrapped error must preserve the ErrNoDefaultIdentity sentinel")
		// The underlying error must also survive so the user sees context.
		assert.ErrorIs(t, err, underlying,
			"wrapped error must also preserve the underlying cause")
	})
}

// TestIdentityFromFlagOrDefault_ErrorReachesFallbackDispatcher is the
// integration contract for every auth subcommand: when GetDefaultIdentity
// fails, the error surfaces through identityFromFlagOrDefault in a form that
// maybeOfferProfileFallbackOnAuthConfigError recognizes and routes to
// MaybeOfferAnyProfileFallback on the AuthManager.
func TestIdentityFromFlagOrDefault_ErrorReachesFallbackDispatcher(t *testing.T) {
	_ = NewTestKit(t)
	resetIdentityViper(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	fallbackInvoked := errors.New("fallback invoked")

	m := types.NewMockAuthManager(ctrl)
	m.EXPECT().GetDefaultIdentity(false).Return("", errors.New("no defaults"))
	// This is the key assertion: the fallback MUST fire for the wrapped error.
	m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(fallbackInvoked)

	cmd := newIdentityFlowTestCmd(t)

	// This mirrors the auth subcommand pattern: resolve the identity,
	// then route any resolution error through
	// maybeOfferProfileFallbackOnAuthConfigError before returning.
	_, err := identityFromFlagOrDefault(cmd, m)
	require.Error(t, err)

	result := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, err)
	require.Error(t, result)
	// Identity equality (assert.Same) is the real contract here — the
	// dispatcher must return the fallback's error instance verbatim.
	// assert.Equal could silently accept an accidental wrap/copy.
	assert.Same(t, fallbackInvoked, result,
		"when the fallback returns an error, it must propagate to the caller unchanged")
}

// TestMaybeOfferProfileFallbackOnAuthConfigError_IgnoresNonAuthErrors is
// the negative companion to
// TestIdentityFromFlagOrDefault_ErrorReachesFallbackDispatcher: when
// the error is not one of the auth-config sentinels
// (ErrNoProvidersAvailable / ErrNoIdentitiesAvailable / ErrNoDefaultIdentity),
// the dispatcher must pass it through verbatim and NEVER invoke
// MaybeOfferAnyProfileFallback.
//
// Gomock's strict "no expectation = fail" semantics make this test
// self-enforcing: any call to the fallback on a non-auth-config
// error fails the test at the gomock controller's Finish().
//
// Also covered in cmd/auth_profile_fallback_test.go as a subtest of
// TestMaybeOfferProfileFallbackOnAuthConfigError — duplicated here
// so the positive and negative paths sit adjacent for readers of
// auth_identity_flow_test.go.
func TestMaybeOfferProfileFallbackOnAuthConfigError_IgnoresNonAuthErrors(t *testing.T) {
	_ = NewTestKit(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	m := types.NewMockAuthManager(ctrl)
	// NO expectation on MaybeOfferAnyProfileFallback — this is the
	// load-bearing assertion. If the dispatcher ever called it for
	// a non-auth-config error, ctrl.Finish() would fail the test.

	result := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, assert.AnError)
	// Identity equality — the dispatcher must return the caller's
	// error instance verbatim (no wrap, no copy).
	assert.Same(t, assert.AnError, result,
		"non-auth-config errors must pass through unchanged without triggering the fallback")
}

// TestAuthCmd_IdentityFlagNoOptDefVal is a tiny regression fence that
// pins the production wiring in cmd/auth.go. The NoOptDefVal test in
// TestIdentityFromFlagOrDefault exercises the NoOptDefVal MECHANISM
// against a test-owned *cobra.Command, so it doesn't catch a future
// edit that deletes the `identityFlag.NoOptDefVal = ...` line from
// cmd/auth.go's init(). This test inspects authCmd directly so that
// specific regression is caught.
func TestAuthCmd_IdentityFlagNoOptDefVal(t *testing.T) {
	flag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
	require.NotNil(t, flag, "--identity must be registered as a persistent flag on authCmd")
	assert.Equal(t, IdentityFlagSelectValue, flag.NoOptDefVal,
		"--identity must have NoOptDefVal = IdentityFlagSelectValue so `atmos auth ... --identity` without a value routes to interactive selection")
}

// TestIdentityFromFlagOrDefault_NoErrorSkipsFallbackDispatcher is the
// negative-path companion to TestIdentityFromFlagOrDefault_
// ErrorReachesFallbackDispatcher: when identityFromFlagOrDefault
// succeeds, every auth subcommand skips the dispatcher entirely via
// the `if err != nil { ... }` guard. Gomock's "no expectation set"
// semantics make this a hard contract — any call to
// MaybeOfferAnyProfileFallback on a successful path would fail the
// test immediately.
func TestIdentityFromFlagOrDefault_NoErrorSkipsFallbackDispatcher(t *testing.T) {
	_ = NewTestKit(t)
	resetIdentityViper(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	m := types.NewMockAuthManager(ctrl)
	m.EXPECT().GetDefaultIdentity(false).Return("default-id", nil)
	// NO expectation on MaybeOfferAnyProfileFallback — gomock's
	// strict mode makes this the load-bearing assertion: any call
	// to the fallback on a success path would fail the test.

	cmd := newIdentityFlowTestCmd(t)

	identityName, err := identityFromFlagOrDefault(cmd, m)
	require.NoError(t, err)
	assert.Equal(t, "default-id", identityName)

	// This is exactly the caller pattern every auth subcommand uses:
	//   identityName, err := identityFromFlagOrDefault(cmd, authManager)
	//   if err != nil {
	//       return maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)
	//   }
	//
	// Since err is nil, the dispatcher must NOT be invoked. We call
	// the dispatcher with nil explicitly to assert the short-circuit
	// at the dispatcher layer: it returns nil without consulting the
	// mock (which has no MaybeOfferAnyProfileFallback expectation).
	result := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, nil)
	assert.NoError(t, result,
		"nil error must return nil from the dispatcher without invoking fallback")
}
