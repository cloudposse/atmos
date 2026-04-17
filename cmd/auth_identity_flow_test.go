package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// newIdentityFlowTestCmd returns a *cobra.Command with only the --identity flag
// registered, enough for identityFromFlagOrDefault to exercise GetIdentityFromFlags.
//
// We drive the helper through Cobra (cmd.Flags().Set) and viper instead of
// mutating os.Args, which would (a) race with any concurrent test and (b) leak
// state across tests because GetIdentityFromFlags reads os.Args directly.
func newIdentityFlowTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringP(IdentityFlagName, "i", "", "")
	return cmd
}

// resetIdentityViper clears any lingering identity flag state in the global
// viper singleton and restores the original value on test completion.
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

	// This is exactly the pattern every auth subcommand uses:
	//   identityName, err := identityFromFlagOrDefault(cmd, authManager)
	//   if err != nil {
	//       return maybeOfferProfileFallbackOnAuthConfigError(ctx, authManager, err)
	//   }
	_, err := identityFromFlagOrDefault(cmd, m)
	require.Error(t, err)

	result := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, err)
	require.Error(t, result)
	assert.Equal(t, fallbackInvoked, result,
		"when the fallback returns an error, it must propagate to the caller unchanged")
}
