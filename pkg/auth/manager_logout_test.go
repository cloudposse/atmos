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
				// With N+1 fix: provider keyring deleted once per identity (2) + once by LogoutProvider = 3 times
				// BUT Delete("test-provider") during identity logout now returns nil (already deleted) instead of error
				store.EXPECT().Delete("test-provider").Return(nil).Times(3)
				store.EXPECT().Delete("identity1").Return(nil)
				store.EXPECT().Delete("identity2").Return(nil)

				// Expect provider logout called ONCE due to N+1 fix (context flag skips it during identity logout).
				provider.EXPECT().Logout(gomock.Any()).Return(nil).Times(1)

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

func TestManager_resolveProviderForIdentity(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		config       *schema.AuthConfig
		want         string
	}{
		{
			name:         "direct provider reference",
			identityName: "identity1",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {
						Via: &schema.IdentityVia{Provider: "provider1"},
					},
				},
			},
			want: "provider1",
		},
		{
			name:         "transitive via identity",
			identityName: "identity2",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {Via: &schema.IdentityVia{Provider: "provider1"}},
					"identity2": {Via: &schema.IdentityVia{Identity: "identity1"}},
				},
			},
			want: "provider1",
		},
		{
			name:         "multi-level transitive chain",
			identityName: "identity3",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {Via: &schema.IdentityVia{Provider: "provider1"}},
					"identity2": {Via: &schema.IdentityVia{Identity: "identity1"}},
					"identity3": {Via: &schema.IdentityVia{Identity: "identity2"}},
				},
			},
			want: "provider1",
		},
		{
			name:         "cycle detection",
			identityName: "identity1",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {Via: &schema.IdentityVia{Identity: "identity2"}},
					"identity2": {Via: &schema.IdentityVia{Identity: "identity1"}},
				},
			},
			want: "",
		},
		{
			name:         "missing identity reference",
			identityName: "identity1",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {Via: &schema.IdentityVia{Identity: "nonexistent"}},
				},
			},
			want: "",
		},
		{
			name:         "no via configuration",
			identityName: "identity1",
			config: &schema.AuthConfig{
				Identities: map[string]schema.Identity{
					"identity1": {},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manager{
				config: tt.config,
			}

			got := m.resolveProviderForIdentity(tt.identityName)
			if got != tt.want {
				t.Errorf("resolveProviderForIdentity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManager_LogoutProvider_TransitiveChain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockProvider := types.NewMockProvider(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	// Config with transitive identity chain: identity3 -> identity2 -> identity1 -> provider1.
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"provider1": {Kind: "aws/sso"},
		},
		Identities: map[string]schema.Identity{
			"identity1": {Via: &schema.IdentityVia{Provider: "provider1"}},
			"identity2": {Via: &schema.IdentityVia{Identity: "identity1"}},
			"identity3": {Via: &schema.IdentityVia{Identity: "identity2"}},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{"provider1": mockProvider},
		identities:      map[string]types.Identity{},
	}

	// Set up mock expectations:
	// - Delete is called for each chain:
	//   identity1: provider1, identity1 (2)
	//   identity2: provider1, identity1, identity2 (3)
	//   identity3: provider1, identity1, identity2, identity3 (4)
	//   LogoutProvider: provider1 (1)
	//   Total: 2 + 3 + 4 + 1 = 10 Delete calls
	//   But credentialStore.Delete treats "not found" as success, so all succeed.
	mockStore.EXPECT().Delete(gomock.Any()).Return(nil).Times(10)
	// - provider.Logout should be called ONCE (not once per identity) due to N+1 fix.
	mockProvider.EXPECT().Logout(gomock.Any()).Return(nil).Times(1)
	// - identity.Logout should be called 3 times (once per identity).
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(nil).Times(3)

	// Register identities.
	for identityName := range config.Identities {
		m.identities[identityName] = mockIdentity
	}

	// Execute LogoutProvider - should find all three identities transitively.
	ctx := context.Background()
	err := m.LogoutProvider(ctx, "provider1")
	if err != nil {
		t.Errorf("LogoutProvider() unexpected error = %v", err)
	}
}

func TestManager_Logout_NotSupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockProvider := types.NewMockProvider(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"github-oidc": {Kind: "github/oidc"},
		},
		Identities: map[string]schema.Identity{
			"github-identity": {Via: &schema.IdentityVia{Provider: "github-oidc"}},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{"github-oidc": mockProvider},
		identities:      map[string]types.Identity{"github-identity": mockIdentity},
	}

	// Mock expectations: Delete keyring, provider returns ErrLogoutNotSupported (treated as success).
	mockStore.EXPECT().Delete("github-identity").Return(nil)
	mockStore.EXPECT().Delete("github-oidc").Return(nil)
	mockProvider.EXPECT().Logout(gomock.Any()).Return(errUtils.ErrLogoutNotSupported)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(nil)

	ctx := context.Background()
	err := m.Logout(ctx, "github-identity")
	// Should succeed (exit 0) even though provider returned ErrLogoutNotSupported.
	if err != nil {
		t.Errorf("Logout() should succeed with ErrLogoutNotSupported, got error = %v", err)
	}
}

func TestManager_LogoutProvider_WithFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockProvider := types.NewMockProvider(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {Kind: "aws/sso"},
		},
		Identities: map[string]schema.Identity{
			"identity1": {Via: &schema.IdentityVia{Provider: "test-provider"}},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{"test-provider": mockProvider},
		identities:      map[string]types.Identity{"identity1": mockIdentity},
	}

	// Mock expectations: Identity logout fails, provider logout fails.
	// Logout identity1: deletes test-provider, identity1.
	mockStore.EXPECT().Delete("test-provider").Return(nil).Times(2) // Once in identity, once in provider.
	mockStore.EXPECT().Delete("identity1").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity logout failed"))
	mockProvider.EXPECT().Logout(gomock.Any()).Return(errors.New("provider logout failed"))

	ctx := context.Background()
	err := m.LogoutProvider(ctx, "test-provider")

	// Should return error with both identity and provider failures.
	if err == nil {
		t.Error("LogoutProvider() should return error when identity and provider logout fail")
	}
	if !errors.Is(err, errUtils.ErrLogoutFailed) {
		t.Errorf("LogoutProvider() error should be ErrLogoutFailed, got: %v", err)
	}
}
