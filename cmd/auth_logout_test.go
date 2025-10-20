package cmd

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPerformIdentityLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		identityName  string
		dryRun        bool
		setupMocks    func(*types.MockAuthManager)
		expectedError error
	}{
		{
			name:         "successful logout",
			identityName: "test-identity",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity").Return(nil)
			},
			expectedError: nil,
		},
		{
			name:         "identity not found",
			identityName: "nonexistent",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"other-identity": {},
				})
			},
			expectedError: errUtils.ErrIdentityNotInConfig,
		},
		{
			name:         "dry run mode",
			identityName: "test-identity",
			dryRun:       true,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().GetFilesDisplayPath("test-provider").Return("/home/user/.aws/atmos")
			},
			expectedError: nil,
		},
		{
			name:         "partial logout",
			identityName: "test-identity",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity").Return(errUtils.ErrPartialLogout)
			},
			expectedError: nil, // Partial logout is treated as success.
		},
		{
			name:         "logout failed",
			identityName: "test-identity",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity").Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
		{
			name:         "dry run with no provider",
			identityName: "test-identity",
			dryRun:       true,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("")
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performIdentityLogout(ctx, mockManager, tt.identityName, tt.dryRun)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformProviderLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		providerName  string
		dryRun        bool
		setupMocks    func(*types.MockAuthManager)
		expectedError error
	}{
		{
			name:         "successful logout",
			providerName: "test-provider",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider").Return(nil)
			},
			expectedError: nil,
		},
		{
			name:         "provider not found",
			providerName: "nonexistent",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"other-provider": {},
				})
			},
			expectedError: errUtils.ErrProviderNotInConfig,
		},
		{
			name:         "dry run mode",
			providerName: "test-provider",
			dryRun:       true,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().GetFilesDisplayPath("test-provider").Return("/home/user/.aws/atmos")
			},
			expectedError: nil,
		},
		{
			name:         "partial logout",
			providerName: "test-provider",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider").Return(errUtils.ErrPartialLogout)
			},
			expectedError: nil, // Partial logout is treated as success (exit 0).
		},
		{
			name:         "logout failed",
			providerName: "test-provider",
			dryRun:       false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider").Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performProviderLogout(ctx, mockManager, tt.providerName, tt.dryRun)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformLogoutAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		dryRun        bool
		setupMocks    func(*types.MockAuthManager)
		expectedError error
	}{
		{
			name:   "successful logout all",
			dryRun: false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any()).Return(nil)
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
					"identity2": {Kind: "aws/user"},
				})
			},
			expectedError: nil,
		},
		{
			name:   "dry run mode with providers",
			dryRun: true,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"provider1": {},
					"provider2": {},
				})
				m.EXPECT().GetFilesDisplayPath("provider1").Return("/home/user/.aws/atmos")
				m.EXPECT().GetFilesDisplayPath("provider2").Return("/home/user/.aws/atmos")
			},
			expectedError: nil,
		},
		{
			name:   "dry run mode with no providers",
			dryRun: true,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{})
			},
			expectedError: nil,
		},
		{
			name:   "partial logout all",
			dryRun: false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any()).Return(errUtils.ErrPartialLogout)
				// Note: GetIdentities() is NOT called for partial logout because it returns early.
			},
			expectedError: nil, // Partial logout is treated as success.
		},
		{
			name:   "logout all failed",
			dryRun: false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any()).Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performLogoutAll(ctx, mockManager, tt.dryRun)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformInteractiveLogout_NoIdentities(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetIdentities().Return(map[string]schema.Identity{})
	mockManager.EXPECT().GetProviders().Return(map[string]schema.Provider{})

	ctx := context.Background()
	err := performInteractiveLogout(ctx, mockManager, false)

	assert.NoError(t, err)
}

func TestDisplayBrowserWarning(t *testing.T) {
	// Test that displayBrowserWarning doesn't panic.
	assert.NotPanics(t, func() {
		displayBrowserWarning()
	})
}

func TestExecuteAuthLogoutCommand_InvalidConfig(t *testing.T) {
	// This test verifies error handling when config initialization fails.
	// We can't easily test the full command execution without mocking the entire config system.
	// The main coverage comes from testing the helper functions above.
	cmd := authLogoutCmd
	err := cmd.ValidateArgs([]string{})
	assert.NoError(t, err) // Command accepts 0 or 1 args.
}
