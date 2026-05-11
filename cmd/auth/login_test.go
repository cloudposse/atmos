package auth

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAuthenticateIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name             string
		setupMock        func(*authTypes.MockAuthManager)
		expectedError    error
		expectedIdent    string
		expectedFallback bool
	}{
		{
			name: "successful authentication with default identity",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("prod-admin", nil)
				m.EXPECT().Authenticate(gomock.Any(), "prod-admin").Return(&authTypes.WhoamiInfo{
					Identity: "prod-admin",
					Provider: "aws-sso",
				}, nil)
			},
			expectedError: nil,
			expectedIdent: "prod-admin",
		},
		{
			name: "authentication failure",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("prod-admin", nil)
				m.EXPECT().Authenticate(gomock.Any(), "prod-admin").Return(nil, errUtils.ErrAuthenticationFailed)
			},
			expectedError: errUtils.ErrAuthenticationFailed,
		},
		{
			name: "no default identity triggers provider fallback",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("", errUtils.ErrNoDefaultIdentity)
			},
			expectedError:    nil,
			expectedFallback: true,
		},
		{
			name: "no identities available triggers provider fallback",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("", errUtils.ErrNoIdentitiesAvailable)
			},
			expectedError:    nil,
			expectedFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuthManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockAuthManager)

			// Create test command.
			cmd := &cobra.Command{Use: "test"}
			// Simulate no identity flag set.
			cmd.Flags().String(IdentityFlagName, "", "identity")

			ctx := context.Background()

			// We need to cast to the interface the function expects.
			whoami, needsFallback, err := authenticateIdentity(ctx, cmd, auth.AuthManager(mockAuthManager))

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFallback, needsFallback)
			if !tt.expectedFallback {
				assert.NotNil(t, whoami)
				assert.Equal(t, tt.expectedIdent, whoami.Identity)
			}
		})
	}
}

func TestAuthLoginCommand_Structure(t *testing.T) {
	assert.Equal(t, "login", authLoginCmd.Use)
	assert.NotEmpty(t, authLoginCmd.Short)
	assert.NotEmpty(t, authLoginCmd.Long)
	assert.NotNil(t, authLoginCmd.RunE)

	// Check provider flag exists.
	providerFlag := authLoginCmd.Flags().Lookup("provider")
	assert.NotNil(t, providerFlag)
	assert.Equal(t, "p", providerFlag.Shorthand)
}

func TestLoginParser_Initialization(t *testing.T) {
	// loginParser should be initialized in init().
	assert.NotNil(t, loginParser)
}

func TestAuthenticateIdentity_WithForceSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthManager := authTypes.NewMockAuthManager(ctrl)

	// Create test command with identity flag set to select value.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String(IdentityFlagName, "", "identity")
	_ = cmd.Flags().Set(IdentityFlagName, IdentityFlagSelectValue)

	// When force select is true, GetDefaultIdentity is called with true.
	mockAuthManager.EXPECT().GetDefaultIdentity(true).Return("selected-identity", nil)
	mockAuthManager.EXPECT().Authenticate(gomock.Any(), "selected-identity").Return(&authTypes.WhoamiInfo{
		Identity: "selected-identity",
		Provider: "aws-sso",
	}, nil)

	ctx := context.Background()
	whoami, _, err := authenticateIdentity(ctx, cmd, auth.AuthManager(mockAuthManager))

	assert.NoError(t, err)
	assert.NotNil(t, whoami)
	assert.Equal(t, "selected-identity", whoami.Identity)
}

func TestAuthLoginCommand_ValidArgsFunction(t *testing.T) {
	// The login command should have a ValidArgsFunction set.
	assert.NotNil(t, authLoginCmd.ValidArgsFunction)
}

func TestAuthLoginCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authLoginCmd.FParseErrWhitelist.UnknownFlags)
}

// fakeProviderLister is a minimal stand-in for the providerLister interface so
// we can exercise getProviderForFallback's branches without spinning up an
// AuthManager that touches the keyring.
type fakeProviderLister struct {
	providers []string
}

func (f *fakeProviderLister) ListProviders() []string { return f.providers }

// TestGetProviderForFallback covers the deterministic branches of the no-
// identities-available auto-provision flow. The interactive prompt branch is
// covered by isInteractive() guards plus integration tests; here we only
// assert the non-interactive paths.
func TestGetProviderForFallback(t *testing.T) {
	t.Run("zero providers returns ErrNoProvidersAvailable", func(t *testing.T) {
		got, err := getProviderForFallback(&fakeProviderLister{providers: nil})
		require.Error(t, err)
		assert.Empty(t, got)
		assert.ErrorIs(t, err, errUtils.ErrNoProvidersAvailable)
	})

	t.Run("single provider auto-selects without prompting", func(t *testing.T) {
		got, err := getProviderForFallback(&fakeProviderLister{providers: []string{"only-sso"}})
		require.NoError(t, err)
		assert.Equal(t, "only-sso", got,
			"a single configured provider must be auto-selected — no prompt, no env var needed")
	})

	t.Run("multiple providers in non-interactive context returns ErrNoDefaultProvider", func(t *testing.T) {
		// Force the non-interactive branch deterministically — otherwise a
		// test runner with a real TTY would block on the huh prompt or be
		// skipped.
		orig := isInteractiveFn
		t.Cleanup(func() { isInteractiveFn = orig })
		isInteractiveFn = func() bool { return false }

		got, err := getProviderForFallback(&fakeProviderLister{providers: []string{"sso-a", "sso-b"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNoDefaultProvider)
		assert.Empty(t, got)
	})
}

// TestIsInteractive asserts the function is callable and returns a bool. The
// actual return value depends on the test runner environment (CI vs local
// terminal vs piped run) so we cannot pin a specific value, but the function
// must not panic and must return a deterministic bool.
func TestIsInteractive(t *testing.T) {
	// First call.
	got := isInteractive()
	// Second call should agree (no hidden state).
	again := isInteractive()
	assert.Equal(t, got, again, "isInteractive() must be deterministic for a given environment")
}

// TestPromptForProvider_EmptyList covers the deterministic guard at the top
// of the interactive prompt: when called with no providers, it returns
// ErrNoProvidersAvailable without ever touching the huh form.
func TestPromptForProvider_EmptyList(t *testing.T) {
	got, err := promptForProvider("Choose a provider:", nil)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.ErrorIs(t, err, errUtils.ErrNoProvidersAvailable)
}

// TestExecuteAuthLoginCommand_SmokeNoConfig exercises the login orchestrator
// from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthLoginCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authLoginCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthLoginCommand(cmd, nil)
	})
}

// TestExecuteAuthLoginCommand_WithMockAuth exercises login end-to-end against
// the mock auth fixture. Drives identity-mode auth (no --provider flag), so
// the orchestrator goes through authenticateIdentity → mock.Authenticate.
func TestExecuteAuthLoginCommand_WithMockAuth(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authLoginCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthLoginCommand(cmd, nil)
	assert.NoError(t, err,
		"login against the mock provider must succeed")
}
