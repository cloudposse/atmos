package okta

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- NewAPIAccessIdentity tests ---

func TestNewAPIAccessIdentity_NilConfig(t *testing.T) {
	_, err := NewAPIAccessIdentity("test", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestNewAPIAccessIdentity_WrongKind(t *testing.T) {
	config := &schema.Identity{Kind: "aws/assume-role"}
	_, err := NewAPIAccessIdentity("test", config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityKind)
}

func TestNewAPIAccessIdentity_Valid(t *testing.T) {
	config := &schema.Identity{
		Kind: "okta/api-access",
		Via:  &schema.IdentityVia{Provider: "okta-corp"},
	}

	identity, err := NewAPIAccessIdentity("test-identity", config)
	require.NoError(t, err)
	assert.Equal(t, "okta/api-access", identity.Kind())
}

// --- Kind tests ---

func TestAPIAccessIdentity_Kind(t *testing.T) {
	identity := &apiAccessIdentity{}
	assert.Equal(t, "okta/api-access", identity.Kind())
}

// --- SetRealm tests ---

func TestAPIAccessIdentity_SetRealm(t *testing.T) {
	identity := &apiAccessIdentity{}
	identity.SetRealm("test-realm")
	assert.Equal(t, "test-realm", identity.realm)
}

// --- GetProviderName tests ---

func TestAPIAccessIdentity_GetProviderName_Valid(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	name, err := identity.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "okta-corp", name)
}

func TestAPIAccessIdentity_GetProviderName_NilVia(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  nil,
		},
	}

	_, err := identity.GetProviderName()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestAPIAccessIdentity_GetProviderName_EmptyProvider(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: ""},
		},
	}

	_, err := identity.GetProviderName()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

// --- Validate tests ---

func TestAPIAccessIdentity_Validate_Valid(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	err := identity.Validate()
	require.NoError(t, err)
}

func TestAPIAccessIdentity_Validate_MissingProvider(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  nil,
		},
	}

	err := identity.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

// --- Authenticate tests ---

func TestAPIAccessIdentity_Authenticate_OktaCredentials(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	creds := &authTypes.OktaCredentials{
		OrgURL:      "https://company.okta.com",
		AccessToken: "test-access-token",
		IDToken:     "test-id-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	result, err := identity.Authenticate(context.Background(), creds)
	require.NoError(t, err)

	// Should return same credentials (pass-through).
	oktaCreds, ok := result.(*authTypes.OktaCredentials)
	require.True(t, ok)
	assert.Equal(t, "test-access-token", oktaCreds.AccessToken)
	assert.Equal(t, "https://company.okta.com", oktaCreds.OrgURL)
}

func TestAPIAccessIdentity_Authenticate_NonOktaCredentials(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	_, err := identity.Authenticate(context.Background(), &mockCredentials{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// --- Environment tests ---

func TestAPIAccessIdentity_Environment(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	env, err := identity.Environment()
	require.NoError(t, err)
	assert.Empty(t, env) // No env vars returned by identity itself.
}

// --- PrepareEnvironment tests ---

func TestAPIAccessIdentity_PrepareEnvironment(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	env := map[string]string{
		"EXISTING": "value",
		"OTHER":    "other",
	}

	result, err := identity.PrepareEnvironment(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, "value", result["EXISTING"])
	assert.Equal(t, "other", result["OTHER"])
	assert.Len(t, result, 2)
}

func TestAPIAccessIdentity_PrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	identity := &apiAccessIdentity{
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	env := map[string]string{"KEY": "value"}
	result, err := identity.PrepareEnvironment(context.Background(), env)
	require.NoError(t, err)

	// Modify result - should not affect original.
	result["NEW"] = "new-value"
	assert.NotContains(t, env, "NEW")
}

// --- Logout tests ---

func TestAPIAccessIdentity_Logout(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	err := identity.Logout(context.Background())
	require.NoError(t, err) // No-op.
}

// --- PostAuthenticate tests ---

func TestAPIAccessIdentity_PostAuthenticate(t *testing.T) {
	tempDir := t.TempDir()

	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	creds := &authTypes.OktaCredentials{
		OrgURL:      "https://company.okta.com",
		AccessToken: "test-access-token",
		IDToken:     "test-id-token",
		ExpiresAt:   time.Now().Add(time.Hour),
		Scope:       "openid profile",
	}

	authCtx := &schema.AuthContext{}
	stackInfo := &schema.ConfigAndStacksInfo{}

	params := &authTypes.PostAuthenticateParams{
		ProviderName: "okta-corp",
		IdentityName: "test-identity",
		Credentials:  creds,
		AuthContext:   authCtx,
		StackInfo:     stackInfo,
		Realm:         "",
	}

	// Override the basePath by pre-creating the Okta file manager directory.
	// We need SetupFiles to use a temp dir. Since it calls NewOktaFileManager with empty basePath,
	// we need to work with what the function does.
	// For this test, we'll verify the function runs without errors.
	// The actual file path will use the home dir, but the function should succeed.
	_ = tempDir

	err := identity.PostAuthenticate(context.Background(), params)
	require.NoError(t, err)

	// Verify auth context was populated.
	require.NotNil(t, authCtx.Okta)
	assert.Equal(t, "https://company.okta.com", authCtx.Okta.OrgURL)
	assert.Equal(t, "test-access-token", authCtx.Okta.AccessToken)
	assert.Equal(t, "test-id-token", authCtx.Okta.IDToken)

	// Verify environment variables were set.
	assert.Equal(t, "https://company.okta.com", stackInfo.ComponentEnvSection["OKTA_ORG_URL"])
	assert.Equal(t, "test-access-token", stackInfo.ComponentEnvSection["OKTA_OAUTH2_ACCESS_TOKEN"])
}

// --- CredentialsExist tests ---

func TestAPIAccessIdentity_CredentialsExist_NoTokens(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "nonexistent-provider-credcheck"},
		},
	}

	exists, err := identity.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestAPIAccessIdentity_CredentialsExist_MissingProvider(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  nil,
		},
	}

	_, err := identity.CredentialsExist()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

// --- Paths tests ---

func TestAPIAccessIdentity_Paths(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	paths, err := identity.Paths()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0].Location, "okta-corp")
	assert.Contains(t, paths[0].Location, "tokens.json")
	assert.Equal(t, authTypes.PathTypeFile, paths[0].Type)
	assert.True(t, paths[0].Required)
	assert.Contains(t, paths[0].Purpose, "test-identity")
}

func TestAPIAccessIdentity_Paths_MissingProvider(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  nil,
		},
	}

	_, err := identity.Paths()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestAPIAccessIdentity_Paths_WithRealm(t *testing.T) {
	identity := &apiAccessIdentity{
		name:  "test-identity",
		realm: "customer-realm",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "okta-corp"},
		},
	}

	paths, err := identity.Paths()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0].Location, "customer-realm")
	assert.Contains(t, paths[0].Location, "okta-corp")
}

// --- LoadCredentials tests ---

func TestAPIAccessIdentity_LoadCredentials_MissingProvider(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  nil,
		},
	}

	_, err := identity.LoadCredentials(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestAPIAccessIdentity_LoadCredentials_NoTokensFile(t *testing.T) {
	identity := &apiAccessIdentity{
		name: "test-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "nonexistent-provider-xyz"},
		},
	}

	_, err := identity.LoadCredentials(context.Background())
	require.Error(t, err)
	// Tokens file won't exist, so should error.
	assert.ErrorIs(t, err, oktaCloud.ErrTokensFileNotFound)
}

func TestAPIAccessIdentity_LoadCredentials_RoundTrip(t *testing.T) {
	// Test by writing tokens to the default location and loading them via the identity.
	// We need to write to the same place the identity will look.
	identity := &apiAccessIdentity{
		name: "test-roundtrip-identity",
		config: &schema.Identity{
			Kind: "okta/api-access",
			Via:  &schema.IdentityVia{Provider: "roundtrip-provider"},
		},
	}

	// Get the path where the identity will look.
	fm, err := oktaCloud.NewOktaFileManager("", "")
	require.NoError(t, err)

	// Write tokens to that path.
	tokens := &oktaCloud.OktaTokens{
		AccessToken:  "roundtrip-access-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
		IDToken:      "roundtrip-id-token",
		RefreshToken: "roundtrip-refresh",
		Scope:        "openid",
	}
	err = fm.WriteTokens("roundtrip-provider", tokens)
	require.NoError(t, err)

	// Ensure cleanup.
	t.Cleanup(func() {
		_ = fm.Cleanup("roundtrip-provider")
	})

	// Now load via the identity.
	creds, err := identity.LoadCredentials(context.Background())
	require.NoError(t, err)

	oktaCreds, ok := creds.(*authTypes.OktaCredentials)
	require.True(t, ok)
	assert.Equal(t, "roundtrip-access-token", oktaCreds.AccessToken)
	assert.Equal(t, "roundtrip-id-token", oktaCreds.IDToken)
	assert.Equal(t, "roundtrip-refresh", oktaCreds.RefreshToken)
	assert.Equal(t, "openid", oktaCreds.Scope)
	// OrgURL should be empty (not stored in tokens file).
	assert.Empty(t, oktaCreds.OrgURL)
}

// --- Interface compliance ---

func TestAPIAccessIdentity_ImplementsIdentityInterface(t *testing.T) {
	var _ authTypes.Identity = (*apiAccessIdentity)(nil)
}

// mockCredentials is a simple mock for non-Okta credential testing.
type mockCredentials struct{}

func (m *mockCredentials) IsExpired() bool                                            { return false }
func (m *mockCredentials) GetExpiration() (*time.Time, error)                         { return nil, nil }
func (m *mockCredentials) BuildWhoamiInfo(_ *authTypes.WhoamiInfo)                    {}
func (m *mockCredentials) Validate(_ context.Context) (*authTypes.ValidationInfo, error) {
	return nil, nil
}
