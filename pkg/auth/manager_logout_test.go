package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManager_Logout(t *testing.T) {
	tests := []struct {
		name          string
		identityName  string
		setupMocks    func(*types.MockCredentialStore, *types.MockProvider, *types.MockIdentity)
		config        *schema.AuthConfig
		wantErr       bool
		wantErrType   error
		checkCalls    bool
		expectedCalls int
	}{
		{
			name:         "successful logout with chain",
			identityName: "test-identity",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Expect keyring deletions for both provider and identity.
				store.EXPECT().Delete("test-provider").Return(nil)
				store.EXPECT().Delete("test-identity").Return(nil)

				// Expect provider logout.
				provider.EXPECT().Logout(gomock.Any()).Return(nil)

				// Expect identity logout.
				identity.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr:       false,
			checkCalls:    true,
			expectedCalls: 2, // Two keyring deletions
		},
		{
			name:         "logout non-existent identity",
			identityName: "non-existent",
			config: &schema.AuthConfig{
				Providers:  map[string]schema.Provider{},
				Identities: map[string]schema.Identity{},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// No calls expected.
			},
			wantErr:     true,
			wantErrType: errUtils.ErrIdentityNotInConfig,
		},
		{
			name:         "partial logout - keyring deletion fails",
			identityName: "test-identity",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// First deletion succeeds, second fails.
				store.EXPECT().Delete("test-provider").Return(nil)
				store.EXPECT().Delete("test-identity").Return(errors.New("keyring error"))

				// Provider and identity logout still called.
				provider.EXPECT().Logout(gomock.Any()).Return(nil)
				identity.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr:     true,
			wantErrType: errUtils.ErrPartialLogout,
		},
		{
			name:         "logout with provider cleanup failure",
			identityName: "test-identity",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Keyring deletions succeed.
				store.EXPECT().Delete("test-provider").Return(nil)
				store.EXPECT().Delete("test-identity").Return(nil)

				// Provider logout fails.
				provider.EXPECT().Logout(gomock.Any()).Return(errors.New("provider cleanup failed"))

				// Identity logout still called.
				identity.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr:     true,
			wantErrType: errUtils.ErrPartialLogout,
		},
		{
			name:         "complete failure - all deletions fail",
			identityName: "test-identity",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"test-identity": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// All keyring deletions fail.
				store.EXPECT().Delete("test-provider").Return(errors.New("keyring error"))
				store.EXPECT().Delete("test-identity").Return(errors.New("keyring error"))

				// Provider and identity logout still called but also fail.
				provider.EXPECT().Logout(gomock.Any()).Return(errors.New("provider error"))
				identity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity error"))
			},
			wantErr:     true,
			wantErrType: errUtils.ErrLogoutFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mocks.
			mockStore := types.NewMockCredentialStore(ctrl)
			mockProvider := types.NewMockProvider(ctrl)
			mockIdentity := types.NewMockIdentity(ctrl)

			// Setup mock expectations.
			tt.setupMocks(mockStore, mockProvider, mockIdentity)

			// Create manager with test configuration.
			m := &manager{
				config:          tt.config,
				providers:       make(map[string]types.Provider),
				identities:      make(map[string]types.Identity),
				credentialStore: mockStore,
			}

			// Add mocked provider and identity if they exist in config.
			if _, exists := tt.config.Providers["test-provider"]; exists {
				m.providers["test-provider"] = mockProvider
			}
			if _, exists := tt.config.Identities["test-identity"]; exists {
				mockIdentity.EXPECT().GetProviderName().Return("test-provider", nil).AnyTimes()
				mockIdentity.EXPECT().Kind().Return("aws/permission-set").AnyTimes()
				m.identities["test-identity"] = mockIdentity
			}

			// Execute logout.
			ctx := context.Background()
			err := m.Logout(ctx, tt.identityName)

			// Check error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("Logout() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check error type if specified.
			if tt.wantErrType != nil && err != nil {
				if !errors.Is(err, tt.wantErrType) {
					t.Errorf("Logout() error type = %v, want error type %v", err, tt.wantErrType)
				}
			}
		})
	}
}

func TestManager_LogoutProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		setupMocks   func(*types.MockCredentialStore, *types.MockProvider, *types.MockIdentity)
		config       *schema.AuthConfig
		wantErr      bool
		wantErrType  error
	}{
		{
			name:         "successful provider logout",
			providerName: "test-provider",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"identity1": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
					"identity2": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "test-provider",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Expect keyring deletions for identities and provider.
				store.EXPECT().Delete("test-provider").Times(3) // Called 3 times: 2 from identities + 1 from provider logout
				store.EXPECT().Delete("identity1").Return(nil)
				store.EXPECT().Delete("identity2").Return(nil)

				// Expect provider logout called multiple times.
				provider.EXPECT().Logout(gomock.Any()).Return(nil).Times(3)

				// Expect identity logout.
				identity.EXPECT().Logout(gomock.Any()).Return(nil).Times(2)
			},
			wantErr: false,
		},
		{
			name:         "logout non-existent provider",
			providerName: "non-existent",
			config: &schema.AuthConfig{
				Providers:  map[string]schema.Provider{},
				Identities: map[string]schema.Identity{},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// No calls expected.
			},
			wantErr:     true,
			wantErrType: errUtils.ErrProviderNotInConfig,
		},
		{
			name:         "provider with no identities",
			providerName: "test-provider",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Only provider cleanup expected.
				store.EXPECT().Delete("test-provider").Return(nil)
				provider.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mocks.
			mockStore := types.NewMockCredentialStore(ctrl)
			mockProvider := types.NewMockProvider(ctrl)
			mockIdentity := types.NewMockIdentity(ctrl)

			// Setup mock expectations.
			tt.setupMocks(mockStore, mockProvider, mockIdentity)

			// Create manager with test configuration.
			m := &manager{
				config:          tt.config,
				providers:       make(map[string]types.Provider),
				identities:      make(map[string]types.Identity),
				credentialStore: mockStore,
			}

			// Add mocked provider if it exists in config.
			if _, exists := tt.config.Providers[tt.providerName]; exists {
				m.providers[tt.providerName] = mockProvider
			}

			// Add mocked identities if they exist in config.
			for identityName := range tt.config.Identities {
				mockIdentity.EXPECT().GetProviderName().Return(tt.providerName, nil).AnyTimes()
				mockIdentity.EXPECT().Kind().Return("aws/permission-set").AnyTimes()
				m.identities[identityName] = mockIdentity
			}

			// Execute logout.
			ctx := context.Background()
			err := m.LogoutProvider(ctx, tt.providerName)

			// Check error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("LogoutProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check error type if specified.
			if tt.wantErrType != nil && err != nil {
				if !errors.Is(err, tt.wantErrType) {
					t.Errorf("LogoutProvider() error type = %v, want error type %v", err, tt.wantErrType)
				}
			}
		})
	}
}

func TestManager_LogoutAll(t *testing.T) {
	tests := []struct {
		name       string
		setupMocks func(*types.MockCredentialStore, *types.MockProvider, *types.MockIdentity)
		config     *schema.AuthConfig
		wantErr    bool
	}{
		{
			name: "successful logout all",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"provider1": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"identity1": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "provider1",
						},
					},
					"identity2": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "provider1",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Expect keyring deletions for all identities and providers.
				store.EXPECT().Delete(gomock.Any()).Return(nil).AnyTimes()

				// Expect provider and identity logout.
				provider.EXPECT().Logout(gomock.Any()).Return(nil).AnyTimes()
				identity.EXPECT().Logout(gomock.Any()).Return(nil).AnyTimes()
			},
			wantErr: false,
		},
		{
			name: "logout all with no identities",
			config: &schema.AuthConfig{
				Providers:  map[string]schema.Provider{},
				Identities: map[string]schema.Identity{},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// No calls expected.
			},
			wantErr: false,
		},
		{
			name: "logout all with partial failures",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"provider1": {
						Kind: "aws/iam-identity-center",
					},
				},
				Identities: map[string]schema.Identity{
					"identity1": {
						Kind: "aws/permission-set",
						Via: &schema.IdentityVia{
							Provider: "provider1",
						},
					},
				},
			},
			setupMocks: func(store *types.MockCredentialStore, provider *types.MockProvider, identity *types.MockIdentity) {
				// Some deletions fail.
				store.EXPECT().Delete(gomock.Any()).Return(errors.New("keyring error")).AnyTimes()

				// Provider and identity logout continue despite failures.
				provider.EXPECT().Logout(gomock.Any()).Return(nil).AnyTimes()
				identity.EXPECT().Logout(gomock.Any()).Return(nil).AnyTimes()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mocks.
			mockStore := types.NewMockCredentialStore(ctrl)
			mockProvider := types.NewMockProvider(ctrl)
			mockIdentity := types.NewMockIdentity(ctrl)

			// Setup mock expectations.
			tt.setupMocks(mockStore, mockProvider, mockIdentity)

			// Create manager with test configuration.
			m := &manager{
				config:          tt.config,
				providers:       make(map[string]types.Provider),
				identities:      make(map[string]types.Identity),
				credentialStore: mockStore,
			}

			// Add mocked providers and identities.
			for providerName := range tt.config.Providers {
				m.providers[providerName] = mockProvider
			}
			for identityName := range tt.config.Identities {
				mockIdentity.EXPECT().GetProviderName().Return("provider1", nil).AnyTimes()
				mockIdentity.EXPECT().Kind().Return("aws/permission-set").AnyTimes()
				m.identities[identityName] = mockIdentity
			}

			// Execute logout.
			ctx := context.Background()
			err := m.LogoutAll(ctx)

			// Check error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("LogoutAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
