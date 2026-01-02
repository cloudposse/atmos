package auth

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAuthenticateIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		setupMock     func(*authTypes.MockAuthManager)
		expectedError error
		expectedIdent string
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
			name: "no default identity",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("", errUtils.ErrNoDefaultIdentity)
			},
			expectedError: errUtils.ErrDefaultIdentity,
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
			whoami, err := authenticateIdentity(ctx, cmd, auth.AuthManager(mockAuthManager))

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
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
	whoami, err := authenticateIdentity(ctx, cmd, auth.AuthManager(mockAuthManager))

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
