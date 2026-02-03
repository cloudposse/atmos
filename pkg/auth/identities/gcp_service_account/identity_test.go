package gcp_service_account

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
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

func (m *MockProvider) Kind() string { return "mock" }
func (m *MockProvider) Name() string { return "mock" }
func (m *MockProvider) SetName(string) {}
func (m *MockProvider) Validate() error                  { return nil }
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
