package gcp_service_account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iamcredentials/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNew(t *testing.T) {
	principal := &types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "test@project.iam.gserviceaccount.com",
	}
	id, err := New(principal)
	require.NoError(t, err)
	require.NotNil(t, id)
	assert.Equal(t, IdentityKind, id.Kind())
}

func TestNew_NilPrincipal(t *testing.T) {
	id, err := New(nil)
	require.Error(t, err)
	assert.Nil(t, id)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestIdentity_Kind(t *testing.T) {
	id := &Identity{principal: &types.GCPServiceAccountIdentityPrincipal{}}
	assert.Equal(t, "gcp/service-account", id.Kind())
}

func TestIdentity_Name(t *testing.T) {
	id := &Identity{principal: &types.GCPServiceAccountIdentityPrincipal{}}
	assert.Equal(t, IdentityKind, id.Name())

	id.SetName("custom-identity")
	assert.Equal(t, "custom-identity", id.Name())
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		principal *types.GCPServiceAccountIdentityPrincipal
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "nil principal",
			principal: nil,
			wantErr:   true,
			errMsg:    "principal is nil",
		},
		{
			name:      "empty email",
			principal: &types.GCPServiceAccountIdentityPrincipal{},
			wantErr:   true,
			errMsg:    "service_account_email is required",
		},
		{
			name: "invalid email format - no @",
			principal: &types.GCPServiceAccountIdentityPrincipal{
				ServiceAccountEmail: "invalid-email",
			},
			wantErr: true,
			errMsg:  "invalid service_account_email format",
		},
		{
			name: "invalid email format - wrong domain",
			principal: &types.GCPServiceAccountIdentityPrincipal{
				ServiceAccountEmail: "sa@gmail.com",
			},
			wantErr: true,
			errMsg:  "invalid service_account_email format",
		},
		{
			name: "valid email",
			principal: &types.GCPServiceAccountIdentityPrincipal{
				ServiceAccountEmail: "deployer@my-project.iam.gserviceaccount.com",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := &Identity{principal: tt.principal}
			err := id.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAuthenticate_NoCredentialsFromProvider(t *testing.T) {
	principal := &types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
	}
	id, _ := New(principal)

	ctx := context.Background()
	creds, err := id.Authenticate(ctx, nil)

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "no credentials from provider")
}

// MockProvider implements types.Provider for testing.
type MockProvider struct {
	creds types.ICredentials
	err   error
}

func (m *MockProvider) Kind() string                            { return "mock" }
func (m *MockProvider) Name() string                            { return "mock" }
func (m *MockProvider) SetName(string)                          {}
func (m *MockProvider) Validate() error                         { return nil }
func (m *MockProvider) PreAuthenticate(types.AuthManager) error { return nil }
func (m *MockProvider) Authenticate(context.Context) (types.ICredentials, error) {
	return m.creds, m.err
}
func (m *MockProvider) Environment() (map[string]string, error) { return nil, nil }
func (m *MockProvider) Paths() ([]types.Path, error)            { return nil, nil }
func (m *MockProvider) PrepareEnvironment(context.Context, map[string]string) (map[string]string, error) {
	return nil, nil
}
func (m *MockProvider) Logout(context.Context) error { return nil }
func (m *MockProvider) GetFilesDisplayPath() string  { return "" }
func (m *MockProvider) SetRealm(string)              {}

func TestAuthenticate_WrongCredentialType(t *testing.T) {
	principal := &types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
	}
	id, _ := New(principal)

	// Pass non-GCP credentials as baseCreds.
	id.SetProvider(&MockProvider{creds: &mockNonGCPCreds{}})
	ctx := context.Background()
	creds, err := id.Authenticate(ctx, &mockNonGCPCreds{})

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "did not return GCP credentials")
}

type mockNonGCPCreds struct{}

func (m *mockNonGCPCreds) IsExpired() bool { return false }
func (m *mockNonGCPCreds) GetExpiration() (*time.Time, error) {
	return nil, nil
}

func (m *mockNonGCPCreds) BuildWhoamiInfo(info *types.WhoamiInfo) {
	if info != nil {
		info.Principal = "mock"
	}
}

func (m *mockNonGCPCreds) Validate(context.Context) (*types.ValidationInfo, error) {
	return nil, nil
}

func TestExtractProjectFromEmail(t *testing.T) {
	tests := []struct {
		email   string
		project string
	}{
		{"sa@my-project.iam.gserviceaccount.com", "my-project"},
		{"deployer@prod-123.iam.gserviceaccount.com", "prod-123"},
		{"invalid", ""},
		{"sa@gmail.com", "gmail.com"}, // Wrong domain, but still parses.
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			assert.Equal(t, tt.project, extractProjectFromEmail(tt.email))
		})
	}
}

func TestGetScopes(t *testing.T) {
	// Default scopes.
	id := &Identity{principal: &types.GCPServiceAccountIdentityPrincipal{}}
	assert.Equal(t, []string{DefaultScope}, id.getScopes())

	// Custom scopes.
	id.principal.Scopes = []string{"scope1", "scope2"}
	assert.Equal(t, []string{"scope1", "scope2"}, id.getScopes())
}

func TestGetLifetime(t *testing.T) {
	// Default lifetime.
	id := &Identity{principal: &types.GCPServiceAccountIdentityPrincipal{}}
	assert.Equal(t, DefaultLifetime, id.getLifetime())

	// Custom lifetime.
	id.principal.Lifetime = "7200s"
	assert.Equal(t, "7200s", id.getLifetime())
}

func TestFormatDelegates(t *testing.T) {
	id := &Identity{principal: &types.GCPServiceAccountIdentityPrincipal{}}

	// No delegates.
	assert.Nil(t, id.formatDelegates())

	// With delegates.
	id.principal.Delegates = []string{
		"intermediate@proj.iam.gserviceaccount.com",
		"other@proj.iam.gserviceaccount.com",
	}
	expected := []string{
		"projects/-/serviceAccounts/intermediate@proj.iam.gserviceaccount.com",
		"projects/-/serviceAccounts/other@proj.iam.gserviceaccount.com",
	}
	assert.Equal(t, expected, id.formatDelegates())
}

func TestEnvironment(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Equal(t, "my-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "my-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "my-project", env["CLOUDSDK_CORE_PROJECT"])
}

func TestEnvironment_ExplicitProjectID(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@other-project.iam.gserviceaccount.com",
			ProjectID:           "explicit-project",
		},
	}

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Equal(t, "explicit-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestDefaultScope(t *testing.T) {
	assert.Equal(t, "https://www.googleapis.com/auth/cloud-platform", DefaultScope)
}

func TestDefaultLifetime(t *testing.T) {
	assert.Equal(t, "3600s", DefaultLifetime)
}

type mockIAMService struct {
	resp     *iamcredentials.GenerateAccessTokenResponse
	err      error
	lastName string
	lastReq  *iamcredentials.GenerateAccessTokenRequest
}

func (m *mockIAMService) GenerateAccessToken(ctx context.Context, name string, req *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	m.lastName = name
	m.lastReq = req
	return m.resp, m.err
}

func TestImpersonateServiceAccount_Success(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
			Scopes:              []string{"scope1"},
			Lifetime:            "1800s",
		},
	}

	mockSvc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "access-token",
			ExpireTime:  time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339),
		},
	}

	id.iamServiceFactory = func(ctx context.Context, accessToken string) (gcpCloud.IAMCredentialsService, error) {
		assert.Equal(t, "upstream-token", accessToken)
		return mockSvc, nil
	}

	token, expiry, err := id.impersonateServiceAccount(context.Background(), "upstream-token")
	require.NoError(t, err)
	assert.Equal(t, "access-token", token)
	assert.False(t, expiry.IsZero())
	assert.Equal(t, "projects/-/serviceAccounts/sa@proj.iam.gserviceaccount.com", mockSvc.lastName)
	require.NotNil(t, mockSvc.lastReq)
	assert.Equal(t, []string{"scope1"}, mockSvc.lastReq.Scope)
	assert.Equal(t, "1800s", mockSvc.lastReq.Lifetime)
}

func TestImpersonateServiceAccount_FactoryError(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		iamServiceFactory: func(ctx context.Context, accessToken string) (gcpCloud.IAMCredentialsService, error) {
			return nil, errors.New("factory error")
		},
	}

	_, _, err := id.impersonateServiceAccount(context.Background(), "upstream-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create IAM credentials service")
}

func TestImpersonateServiceAccount_ServiceError(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}

	mockSvc := &mockIAMService{err: errors.New("svc error")}
	id.iamServiceFactory = func(ctx context.Context, accessToken string) (gcpCloud.IAMCredentialsService, error) {
		return mockSvc, nil
	}

	_, _, err := id.impersonateServiceAccount(context.Background(), "upstream-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generate access token")
}

func TestGetProviderName_WithConfig(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}

	// Without config, returns empty string.
	name, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "", name)

	// With config containing Via.Provider, returns the provider name.
	id.SetConfig(&schema.Identity{
		Kind: "gcp/service-account",
		Via: &schema.IdentityVia{
			Provider: "my-gcp-adc",
		},
	})
	name, err = id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "my-gcp-adc", name)
}

func TestGetProviderName_WithProvider(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}

	// Set provider instance (fallback when config is not set).
	id.SetProvider(&MockProvider{})
	name, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "mock", name)

	// Config takes precedence over provider instance.
	id.SetConfig(&schema.Identity{
		Via: &schema.IdentityVia{
			Provider: "config-provider",
		},
	})
	name, err = id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "config-provider", name)
}

func TestPrepareEnvironment_NoCredentials(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	id := &Identity{
		name:  "test-identity-no-creds",
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		config: &schema.Identity{
			Via: &schema.IdentityVia{
				Provider: "gcp-adc",
			},
		},
	}

	ctx := context.Background()
	env, err := id.PrepareEnvironment(ctx, map[string]string{"PATH": "/usr/bin"})

	// Should error because no credentials exist.
	require.Error(t, err)
	assert.Nil(t, env)
	assert.Contains(t, err.Error(), "no valid credentials found")
	assert.Contains(t, err.Error(), "test-identity-no-creds")
	assert.Contains(t, err.Error(), "atmos auth login")
}

func TestSetRealm(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	id.SetRealm("custom-realm")
	assert.Equal(t, "custom-realm", id.realm)
}

func TestRequireRealm_Empty(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	id.realm = ""
	realm, err := id.requireRealm()
	require.Error(t, err)
	assert.Empty(t, realm)
	assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
}

func TestRequireRealm_Set(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	id.realm = "custom"
	realm, err := id.requireRealm()
	require.NoError(t, err)
	assert.Equal(t, "custom", realm)
}

func TestPaths(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestAuthenticate_Success(t *testing.T) {
	principal := &types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
		Scopes:              []string{"scope1"},
		Lifetime:            "1800s",
	}
	id, err := New(principal)
	require.NoError(t, err)

	expiry := time.Now().Add(30 * time.Minute).UTC()
	mockSvc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "impersonated-token",
			ExpireTime:  expiry.Format(time.RFC3339),
		},
	}
	id.iamServiceFactory = func(ctx context.Context, accessToken string) (gcpCloud.IAMCredentialsService, error) {
		return mockSvc, nil
	}

	baseCreds := &types.GCPCredentials{
		AccessToken: "upstream-token",
		TokenExpiry: time.Now().Add(time.Hour),
	}
	result, err := id.Authenticate(context.Background(), baseCreds)
	require.NoError(t, err)
	require.NotNil(t, result)

	gcpCreds, ok := result.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "impersonated-token", gcpCreds.AccessToken)
	assert.Equal(t, "sa@my-project.iam.gserviceaccount.com", gcpCreds.ServiceAccountEmail)
	assert.Equal(t, "my-project", gcpCreds.ProjectID)
}

func TestAuthenticate_ExplicitProjectID(t *testing.T) {
	principal := &types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "sa@other-project.iam.gserviceaccount.com",
		ProjectID:           "explicit-project",
	}
	id, err := New(principal)
	require.NoError(t, err)

	mockSvc := &mockIAMService{
		resp: &iamcredentials.GenerateAccessTokenResponse{
			AccessToken: "token",
			ExpireTime:  time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
	}
	id.iamServiceFactory = func(ctx context.Context, accessToken string) (gcpCloud.IAMCredentialsService, error) {
		return mockSvc, nil
	}

	baseCreds := &types.GCPCredentials{AccessToken: "upstream"}
	result, err := id.Authenticate(context.Background(), baseCreds)
	require.NoError(t, err)

	gcpCreds := result.(*types.GCPCredentials)
	assert.Equal(t, "explicit-project", gcpCreds.ProjectID)
}

func TestPostAuthenticate_NilParams(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	err := id.PostAuthenticate(context.Background(), nil)
	assert.NoError(t, err)
}

func TestPostAuthenticate_NilCredentials(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{})
	assert.NoError(t, err)
}

func TestPostAuthenticate_WrongCredentialType(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		Credentials: &mockNonGCPCreds{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected GCP credentials")
}

func TestPostAuthenticate_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	id := &Identity{
		name:  "test-sa",
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}

	creds := &types.GCPCredentials{
		AccessToken: "test-token",
		TokenExpiry: time.Now().Add(time.Hour),
		ProjectID:   "test-project",
	}
	authCtx := &schema.AuthContext{}
	err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		Credentials:  creds,
		ProviderName: "gcp-adc",
		AuthContext:  authCtx,
	})
	require.NoError(t, err)
	require.NotNil(t, authCtx.GCP)
	assert.Equal(t, "test-project", authCtx.GCP.ProjectID)
}

func TestLogout_NoProvider(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	err := id.Logout(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider name is required")
}

func TestLogout_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	id := &Identity{
		name:  "test-sa",
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		config: &schema.Identity{
			Via: &schema.IdentityVia{
				Provider: "gcp-adc",
			},
		},
	}
	err := id.Logout(context.Background())
	assert.NoError(t, err)
}

func TestCredentialsExist_NoProvider(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	exists, err := id.CredentialsExist()
	require.Error(t, err)
	assert.False(t, exists)
}

func TestCredentialsExist_NoCreds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	id := &Identity{
		name:  "cred-check-identity",
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		config: &schema.Identity{
			Via: &schema.IdentityVia{
				Provider: "gcp-adc",
			},
		},
	}
	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestLoadCredentials_NoProvider(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
	}
	creds, err := id.LoadCredentials(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
}

func TestLoadCredentials_NoCreds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	id := &Identity{
		name:  "load-test-identity",
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		config: &schema.Identity{
			Via: &schema.IdentityVia{
				Provider: "gcp-adc",
			},
		},
	}
	creds, err := id.LoadCredentials(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrNoCredentialsFound)
}

func TestPrepareEnvironment_NoProviderName(t *testing.T) {
	id := &Identity{
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
		},
		realm: "test-realm",
	}
	_, err := id.PrepareEnvironment(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider name is required")
}

// TestEmptyRealm_RejectedByFileOperations verifies that all credential file
// operations fail fast with ErrEmptyRealm when realm has not been set.
func TestEmptyRealm_RejectedByFileOperations(t *testing.T) {
	makeIdentity := func() *Identity {
		return &Identity{
			name: "test-sa",
			// realm intentionally left empty.
			principal: &types.GCPServiceAccountIdentityPrincipal{
				ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
			},
			config: &schema.Identity{
				Via: &schema.IdentityVia{
					Provider: "gcp-adc",
				},
			},
		}
	}

	t.Run("PrepareEnvironment", func(t *testing.T) {
		id := makeIdentity()
		_, err := id.PrepareEnvironment(context.Background(), nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
	})

	t.Run("PostAuthenticate", func(t *testing.T) {
		id := makeIdentity()
		err := id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
			Credentials:  &types.GCPCredentials{AccessToken: "tok"},
			ProviderName: "gcp-adc",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
	})

	t.Run("Logout", func(t *testing.T) {
		id := makeIdentity()
		err := id.Logout(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
	})

	t.Run("CredentialsExist", func(t *testing.T) {
		id := makeIdentity()
		_, err := id.CredentialsExist()
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
	})

	t.Run("LoadCredentials", func(t *testing.T) {
		id := makeIdentity()
		_, err := id.LoadCredentials(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrEmptyRealm)
	})
}

// TestRealmIsolation_DistinctPaths verifies that two identities with the same
// name and provider but different realms produce distinct credential file paths.
func TestRealmIsolation_DistinctPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	makeIdentityWithRealm := func(realmValue string) *Identity {
		return &Identity{
			name:  "shared-identity",
			realm: realmValue,
			principal: &types.GCPServiceAccountIdentityPrincipal{
				ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
			},
			config: &schema.Identity{
				Via: &schema.IdentityVia{
					Provider: "shared-provider",
				},
			},
		}
	}

	id1 := makeIdentityWithRealm("realm-alpha")
	id2 := makeIdentityWithRealm("realm-beta")

	creds := &types.GCPCredentials{
		AccessToken: "test-token",
		TokenExpiry: time.Now().Add(time.Hour),
		ProjectID:   "proj",
	}

	// Write credentials for both realms.
	err := id1.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		Credentials:  creds,
		ProviderName: "shared-provider",
		AuthContext:   &schema.AuthContext{},
	})
	require.NoError(t, err)

	err = id2.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		Credentials:  creds,
		ProviderName: "shared-provider",
		AuthContext:   &schema.AuthContext{},
	})
	require.NoError(t, err)

	// Both must have credentials.
	exists1, err := id1.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists1, "realm-alpha should have credentials")

	exists2, err := id2.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists2, "realm-beta should have credentials")

	// Cleaning up realm-alpha must not affect realm-beta.
	err = id1.Logout(context.Background())
	require.NoError(t, err)

	exists1After, err := id1.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists1After, "realm-alpha should have no credentials after logout")

	exists2After, err := id2.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists2After, "realm-beta should still have credentials after realm-alpha logout")
}

// TestNewIdentity_NoDefaultRealm verifies that newly constructed identities
// do not have a default realm — it must be explicitly set via SetRealm.
func TestNewIdentity_NoDefaultRealm(t *testing.T) {
	id, err := New(&types.GCPServiceAccountIdentityPrincipal{
		ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
	})
	require.NoError(t, err)
	assert.Empty(t, id.realm, "new identity must not have a default realm")
}
