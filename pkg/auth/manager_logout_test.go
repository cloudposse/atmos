package auth

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

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
				// Expect keyring deletion for identity only (provider is preserved).
				store.EXPECT().Delete("test-identity").Return(nil)

				// Expect identity logout (provider logout not called for identity logout).
				identity.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr:       false,
			checkCalls:    true,
			expectedCalls: 1, // One keyring deletion (identity only)
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
				// Keyring deletion fails for identity.
				store.EXPECT().Delete("test-identity").Return(errors.New("keyring error"))

				// Identity logout still called (provider logout NOT called for identity logout).
				identity.EXPECT().Logout(gomock.Any()).Return(nil)
			},
			wantErr:     true,
			wantErrType: errUtils.ErrPartialLogout,
		},
		{
			name:         "logout with identity cleanup failure",
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
				// Keyring deletion succeeds.
				store.EXPECT().Delete("test-identity").Return(nil)

				// Identity logout fails (provider logout NOT called for identity logout).
				identity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity cleanup failed"))
			},
			wantErr:     true,
			wantErrType: errUtils.ErrPartialLogout,
		},
		{
			name:         "complete failure - all operations fail",
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
				// Keyring deletion fails.
				store.EXPECT().Delete("test-identity").Return(errors.New("keyring error"))

				// Identity logout also fails (provider logout NOT called for identity logout).
				identity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity error"))
			},
			wantErr:     true,
			wantErrType: errUtils.ErrPartialLogout,
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

			// Execute logout (with deleteKeychain=true to match test expectations).
			ctx := context.Background()
			err := m.Logout(ctx, tt.identityName, true)

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
				// Expect keyring deletions for identities (identity logout only deletes identity keyring).
				store.EXPECT().Delete("identity1").Return(nil)
				store.EXPECT().Delete("identity2").Return(nil)

				// LogoutProvider deletes provider keyring once.
				store.EXPECT().Delete("test-provider").Return(nil)

				// Expect provider logout called once by LogoutProvider.
				provider.EXPECT().Logout(gomock.Any()).Return(nil)

				// Expect identity logout called once per identity.
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

			// Execute logout (with deleteKeychain=true to match test expectations).
			ctx := context.Background()
			err := m.LogoutProvider(ctx, tt.providerName, true)

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

			// Execute logout (with deleteKeychain=true to match test expectations).
			ctx := context.Background()
			err := m.LogoutAll(ctx, true)

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
	// - Delete is called for each identity (identity logout only deletes identity keyring):
	//   identity1: identity1 (1)
	//   identity2: identity2 (1)
	//   identity3: identity3 (1)
	//   LogoutProvider: provider1 (1)
	//   Total: 4 Delete calls
	mockStore.EXPECT().Delete("identity1").Return(nil)
	mockStore.EXPECT().Delete("identity2").Return(nil)
	mockStore.EXPECT().Delete("identity3").Return(nil)
	mockStore.EXPECT().Delete("provider1").Return(nil)
	// - provider.Logout should be called ONCE by LogoutProvider.
	mockProvider.EXPECT().Logout(gomock.Any()).Return(nil)
	// - identity.Logout should be called 3 times (once per identity).
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(nil).Times(3)

	// Register identities.
	for identityName := range config.Identities {
		m.identities[identityName] = mockIdentity
	}

	// Execute LogoutProvider - should find all three identities transitively (deleteKeychain=true).
	ctx := context.Background()
	err := m.LogoutProvider(ctx, "provider1", true)
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

	// Mock expectations: Delete keyring for identity only, identity returns ErrLogoutNotSupported (treated as success).
	mockStore.EXPECT().Delete("github-identity").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(errUtils.ErrLogoutNotSupported)

	ctx := context.Background()
	err := m.Logout(ctx, "github-identity", true)
	// Should succeed (exit 0) even though identity returned ErrLogoutNotSupported.
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
	// Logout identity1: deletes identity1 only.
	// LogoutProvider: deletes test-provider.
	mockStore.EXPECT().Delete("identity1").Return(nil)
	mockStore.EXPECT().Delete("test-provider").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity logout failed"))
	mockProvider.EXPECT().Logout(gomock.Any()).Return(errors.New("provider logout failed"))

	ctx := context.Background()
	err := m.LogoutProvider(ctx, "test-provider", true)

	// Should return error with both identity and provider failures.
	if err == nil {
		t.Error("LogoutProvider() should return error when identity and provider logout fail")
	}
	if !errors.Is(err, errUtils.ErrLogoutFailed) {
		t.Errorf("LogoutProvider() error should be ErrLogoutFailed, got: %v", err)
	}
}

func TestManager_Logout_IdentityInChain(t *testing.T) {
	// Test the scenario where the first element in chain is an identity, not a provider.
	// This exercises lines 789-803 in manager.go.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	// Create config with standalone identity (no provider).
	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"standalone-identity": {
				Kind: "aws/user",
			},
		},
	}

	identityMap := map[string]types.Identity{
		"standalone-identity": mockIdentity,
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{},
		identities:      identityMap,
	}

	// Mock expectations.
	mockStore.EXPECT().Delete("standalone-identity").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(nil)

	ctx := context.Background()
	err := m.Logout(ctx, "standalone-identity", true)
	if err != nil {
		t.Errorf("Logout() failed: %v", err)
	}
}

func TestManager_Logout_IdentityLogoutNotSupported(t *testing.T) {
	// Test identity.Logout returning ErrLogoutNotSupported (should be treated as success).
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind: "aws/user",
			},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{},
		identities: map[string]types.Identity{
			"test-identity": mockIdentity,
		},
	}

	// Mock expectations - identity returns ErrLogoutNotSupported.
	mockStore.EXPECT().Delete("test-identity").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(errUtils.ErrLogoutNotSupported)

	ctx := context.Background()
	err := m.Logout(ctx, "test-identity", true)
	// Should succeed (ErrLogoutNotSupported is treated as success).
	if err != nil {
		t.Errorf("Logout() should succeed when identity.Logout returns ErrLogoutNotSupported, got: %v", err)
	}
}

func TestManager_Logout_IdentityInChainLogoutFails(t *testing.T) {
	// Test identity in chain returning an error.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockIdentity := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"standalone-identity": {
				Kind: "aws/user",
			},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers:       map[string]types.Provider{},
		identities: map[string]types.Identity{
			"standalone-identity": mockIdentity,
		},
	}

	// Mock expectations - identity logout fails.
	mockStore.EXPECT().Delete("standalone-identity").Return(nil)
	mockIdentity.EXPECT().Logout(gomock.Any()).Return(errors.New("identity cleanup failed"))

	ctx := context.Background()
	err := m.Logout(ctx, "standalone-identity", true)

	// Should return partial logout (1 keyring deleted, but identity cleanup failed).
	if err == nil {
		t.Error("Logout() should return error when identity logout fails")
	}
	if !errors.Is(err, errUtils.ErrPartialLogout) {
		t.Errorf("Logout() should return ErrPartialLogout, got: %v", err)
	}
}

func TestManager_LogoutAll_WithErrors(t *testing.T) {
	// Test LogoutAll with multiple identities and some failures.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockProvider := types.NewMockProvider(ctrl)
	mockIdentity1 := types.NewMockIdentity(ctrl)
	mockIdentity2 := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"provider1": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "provider1"},
			},
			"identity2": {
				Kind: "aws/user",
			},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers: map[string]types.Provider{
			"provider1": mockProvider,
		},
		identities: map[string]types.Identity{
			"identity1": mockIdentity1,
			"identity2": mockIdentity2,
		},
	}

	// Mock expectations for identity1 (Logout only deletes identity keyring).
	mockStore.EXPECT().Delete("identity1").Return(nil)
	mockIdentity1.EXPECT().Logout(gomock.Any()).Return(nil)

	// Mock expectations for identity2 - keyring deletion fails.
	mockStore.EXPECT().Delete("identity2").Return(errors.New("keyring error"))
	mockIdentity2.EXPECT().Logout(gomock.Any()).Return(nil)

	// Mock expectations for provider logout (LogoutAll now logs out providers too).
	mockStore.EXPECT().Delete("provider1").Return(nil)
	mockProvider.EXPECT().Logout(gomock.Any()).Return(nil)

	ctx := context.Background()
	err := m.LogoutAll(ctx, true)

	// Should return error when some deletions fail.
	// Since identity2 has 0 removed and 1 error, it returns ErrLogoutFailed.
	// The overall LogoutAll then returns ErrLogoutFailed because at least one identity failed.
	if err == nil {
		t.Error("LogoutAll() should return error when some deletions fail")
	}
	if !errors.Is(err, errUtils.ErrLogoutFailed) {
		t.Errorf("LogoutAll() should return ErrLogoutFailed, got: %v", err)
	}
}

func TestManager_LogoutAll_LogsOutProviders(t *testing.T) {
	// Test that LogoutAll explicitly logs out providers in addition to identities.
	// This test formalizes the bug fix where --all was leaving orphaned provider credentials.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := types.NewMockCredentialStore(ctrl)
	mockProvider1 := types.NewMockProvider(ctrl)
	mockProvider2 := types.NewMockProvider(ctrl)
	mockIdentity1 := types.NewMockIdentity(ctrl)
	mockIdentity2 := types.NewMockIdentity(ctrl)

	config := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"provider1": {Kind: "aws/iam-identity-center"},
			"provider2": {Kind: "aws/iam-identity-center"},
		},
		Identities: map[string]schema.Identity{
			"identity1": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "provider1"},
			},
			"identity2": {
				Kind: "aws/permission-set",
				Via:  &schema.IdentityVia{Provider: "provider2"},
			},
		},
	}

	m := &manager{
		config:          config,
		credentialStore: mockStore,
		providers: map[string]types.Provider{
			"provider1": mockProvider1,
			"provider2": mockProvider2,
		},
		identities: map[string]types.Identity{
			"identity1": mockIdentity1,
			"identity2": mockIdentity2,
		},
	}

	// Expect identity logout calls.
	mockStore.EXPECT().Delete("identity1").Return(nil)
	mockIdentity1.EXPECT().Logout(gomock.Any()).Return(nil)
	mockStore.EXPECT().Delete("identity2").Return(nil)
	mockIdentity2.EXPECT().Logout(gomock.Any()).Return(nil)

	// CRITICAL: Expect provider logout calls (this is what the bug fix added).
	// Both provider keyring entries should be deleted.
	mockStore.EXPECT().Delete("provider1").Return(nil)
	mockProvider1.EXPECT().Logout(gomock.Any()).Return(nil)
	mockStore.EXPECT().Delete("provider2").Return(nil)
	mockProvider2.EXPECT().Logout(gomock.Any()).Return(nil)

	ctx := context.Background()
	err := m.LogoutAll(ctx, true)
	if err != nil {
		t.Errorf("LogoutAll() should succeed when all operations succeed, got: %v", err)
	}

	// Verify the test would fail if provider logout wasn't called.
	// The gomock controller will automatically fail if expected calls aren't made.
}
