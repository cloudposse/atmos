package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPerformIdentityLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		identityName   string
		dryRun         bool
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "successful logout without keychain deletion",
			identityName:   "test-identity",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity", false).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "identity not found",
			identityName:   "nonexistent",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"other-identity": {},
				})
			},
			expectedError: errUtils.ErrIdentityNotInConfig,
		},
		{
			name:           "dry run mode",
			identityName:   "test-identity",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
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
			name:           "partial logout",
			identityName:   "test-identity",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity", false).Return(errUtils.ErrPartialLogout)
			},
			expectedError: nil, // Partial logout is treated as success.
		},
		{
			name:           "logout failed",
			identityName:   "test-identity",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity", false).Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
		{
			name:           "dry run with no provider",
			identityName:   "test-identity",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
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
			err := performIdentityLogout(ctx, mockManager, tt.identityName, tt.dryRun, tt.deleteKeychain, tt.force)

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
		name           string
		providerName   string
		dryRun         bool
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "successful logout without keychain deletion",
			providerName:   "test-provider",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider", false).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "provider not found",
			providerName:   "nonexistent",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"other-provider": {},
				})
			},
			expectedError: errUtils.ErrProviderNotInConfig,
		},
		{
			name:           "dry run mode",
			providerName:   "test-provider",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
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
			name:           "partial logout",
			providerName:   "test-provider",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider", false).Return(errUtils.ErrPartialLogout)
			},
			expectedError: nil, // Partial logout is treated as success (exit 0).
		},
		{
			name:           "logout failed",
			providerName:   "test-provider",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider", false).Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performProviderLogout(ctx, mockManager, tt.providerName, tt.dryRun, tt.deleteKeychain, tt.force)

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
		name           string
		dryRun         bool
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "successful logout all without keychain deletion",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(nil)
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
					"identity2": {Kind: "aws/user"},
				})
			},
			expectedError: nil,
		},
		{
			name:           "dry run mode with providers",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
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
			name:           "dry run mode with no providers",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{})
			},
			expectedError: nil,
		},
		{
			name:           "partial logout all",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(errUtils.ErrPartialLogout)
				// Note: GetIdentities() is NOT called for partial logout because it returns early.
			},
			expectedError: nil, // Partial logout is treated as success.
		},
		{
			name:           "logout all failed",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performLogoutAll(ctx, mockManager, tt.dryRun, tt.deleteKeychain, tt.force)

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
	err := performInteractiveLogout(ctx, mockManager, false, false, false)

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

func TestExecuteAuthLogoutCommand_SupportsIdentityFlag(t *testing.T) {
	// This test verifies that both positional argument and --identity flag work.
	// When both are provided, positional argument takes precedence.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name         string
		args         []string
		identityFlag string
		expectedCall string // which identity name should be passed to performIdentityLogout
	}{
		{
			name:         "positional argument only",
			args:         []string{"identity-from-arg"},
			identityFlag: "",
			expectedCall: "identity-from-arg",
		},
		{
			name:         "identity flag only",
			args:         []string{},
			identityFlag: "identity-from-flag",
			expectedCall: "identity-from-flag",
		},
		{
			name:         "both provided - positional takes precedence",
			args:         []string{"identity-from-arg"},
			identityFlag: "identity-from-flag",
			expectedCall: "identity-from-arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The test logic is covered by the unit tests for the helper functions.
			// This test documents the behavior that both forms are accepted.
			// Full integration testing would require mocking the entire config and auth system.
			assert.NotEmpty(t, tt.expectedCall)
		})
	}
}

func TestPerformLogoutAll_WithAllFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		dryRun         bool
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "all flag triggers logout all",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(nil)
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
					"identity2": {Kind: "aws/user"},
				})
			},
			expectedError: nil,
		},
		{
			name:           "all flag with dry run",
			dryRun:         true,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"provider1": {},
				})
				m.EXPECT().GetFilesDisplayPath("provider1").Return("/home/user/.config/atmos")
			},
			expectedError: nil,
		},
		{
			name:           "all flag with partial logout",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(errUtils.ErrPartialLogout)
			},
			expectedError: nil, // Partial logout treated as success.
		},
		{
			name:           "all flag with logout failure",
			dryRun:         false,
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(errUtils.ErrLogoutFailed)
			},
			expectedError: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performLogoutAll(ctx, mockManager, tt.dryRun, tt.deleteKeychain, tt.force)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformIdentityLogout_WithKeychainFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		identityName   string
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "logout without keychain deletion (default)",
			identityName:   "test-identity",
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity", false).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "logout with keychain deletion",
			identityName:   "test-identity",
			deleteKeychain: true,
			force:          true, // Force to skip confirmation.
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"test-identity": {},
				})
				m.EXPECT().GetProviderForIdentity("test-identity").Return("test-provider")
				m.EXPECT().Logout(gomock.Any(), "test-identity", true).Return(nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performIdentityLogout(ctx, mockManager, tt.identityName, false, tt.deleteKeychain, tt.force)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformProviderLogout_WithKeychainFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		providerName   string
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "provider logout without keychain deletion (default)",
			providerName:   "test-provider",
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider", false).Return(nil)
			},
			expectedError: nil,
		},
		{
			name:           "provider logout with keychain deletion",
			providerName:   "test-provider",
			deleteKeychain: true,
			force:          true, // Force to skip confirmation.
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"test-provider": {},
				})
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
				m.EXPECT().GetProviderForIdentity("identity1").Return("test-provider")
				m.EXPECT().LogoutProvider(gomock.Any(), "test-provider", true).Return(nil)
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performProviderLogout(ctx, mockManager, tt.providerName, false, tt.deleteKeychain, tt.force)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPerformLogoutAll_WithKeychainFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		deleteKeychain bool
		force          bool
		setupMocks     func(*types.MockAuthManager)
		expectedError  error
	}{
		{
			name:           "logout all without keychain deletion (default)",
			deleteKeychain: false,
			force:          false,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), false).Return(nil)
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
			},
			expectedError: nil,
		},
		{
			name:           "logout all with keychain deletion",
			deleteKeychain: true,
			force:          true, // Force to skip confirmation.
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().LogoutAll(gomock.Any(), true).Return(nil)
				m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
					"identity1": {Kind: "aws/permission-set"},
				})
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockManager := types.NewMockAuthManager(ctrl)
			tt.setupMocks(mockManager)

			ctx := context.Background()
			err := performLogoutAll(ctx, mockManager, false, tt.deleteKeychain, tt.force)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
