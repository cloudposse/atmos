package okta

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetupFiles_WritesTokens(t *testing.T) {
	tempDir := t.TempDir()

	creds := &types.OktaCredentials{
		OrgURL:                "https://company.okta.com",
		AccessToken:           "test-access-token",
		IDToken:               "test-id-token",
		RefreshToken:          "test-refresh-token",
		ExpiresAt:             time.Now().Add(time.Hour),
		RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Scope:                 "openid profile",
	}

	err := SetupFiles("test-provider", "test-identity", creds, tempDir, "")
	require.NoError(t, err)

	// Verify tokens were written by loading them back.
	mgr, err := NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	assert.True(t, mgr.TokensExist("test-provider"))

	tokens, err := mgr.LoadTokens("test-provider")
	require.NoError(t, err)

	assert.Equal(t, "test-access-token", tokens.AccessToken)
	assert.Equal(t, "Bearer", tokens.TokenType)
	assert.Equal(t, "test-id-token", tokens.IDToken)
	assert.Equal(t, "test-refresh-token", tokens.RefreshToken)
	assert.Equal(t, "openid profile", tokens.Scope)
}

func TestSetupFiles_NonOktaCredentials(t *testing.T) {
	// When credentials are not OktaCredentials, SetupFiles should be a no-op.
	err := SetupFiles("test-provider", "test-identity", &mockCredentials{}, "/tmp/test", "")
	require.NoError(t, err)
}

func TestSetupFiles_NilCredentials(t *testing.T) {
	// When credentials are nil, SetupFiles returns nil (graceful no-op).
	err := SetupFiles("test-provider", "test-identity", nil, "/tmp/test", "")
	require.NoError(t, err)
}

func TestSetupFiles_WithRealm(t *testing.T) {
	tempDir := t.TempDir()

	creds := &types.OktaCredentials{
		OrgURL:      "https://company.okta.com",
		AccessToken: "test-access-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	err := SetupFiles("test-provider", "test-identity", creds, tempDir, "test-realm")
	require.NoError(t, err)

	// Custom basePath is used directly, realm is not appended.
	mgr, err := NewOktaFileManager(tempDir, "test-realm")
	require.NoError(t, err)
	assert.True(t, mgr.TokensExist("test-provider"))
}

func TestSetAuthContext_NilParams(t *testing.T) {
	err := SetAuthContext(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestSetAuthContext_NilAuthContext(t *testing.T) {
	params := &SetAuthContextParams{
		AuthContext: nil,
	}
	err := SetAuthContext(params)
	require.NoError(t, err) // Graceful no-op.
}

func TestSetAuthContext_NonOktaCredentials(t *testing.T) {
	authCtx := &schema.AuthContext{}
	params := &SetAuthContextParams{
		AuthContext:  authCtx,
		Credentials:  &mockCredentials{},
		ProviderName: "test-provider",
	}
	err := SetAuthContext(params)
	require.NoError(t, err)
	assert.Nil(t, authCtx.Okta) // No Okta context set.
}

func TestSetAuthContext_ExpiredCredentials(t *testing.T) {
	authCtx := &schema.AuthContext{}
	creds := &types.OktaCredentials{
		OrgURL:      "https://company.okta.com",
		AccessToken: "expired-token",
		ExpiresAt:   time.Now().Add(-time.Hour),
	}

	params := &SetAuthContextParams{
		AuthContext:  authCtx,
		Credentials:  creds,
		ProviderName: "test-provider",
		BasePath:     t.TempDir(),
	}
	err := SetAuthContext(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestSetAuthContext_PopulatesContext(t *testing.T) {
	tempDir := t.TempDir()
	authCtx := &schema.AuthContext{}
	creds := &types.OktaCredentials{
		OrgURL:      "https://company.okta.com",
		AccessToken: "test-access-token",
		IDToken:     "test-id-token",
		ExpiresAt:   time.Now().Add(time.Hour),
	}

	params := &SetAuthContextParams{
		AuthContext:   authCtx,
		ProviderName:  "test-provider",
		IdentityName:  "test-identity",
		Credentials:   creds,
		BasePath:      tempDir,
	}

	err := SetAuthContext(params)
	require.NoError(t, err)

	require.NotNil(t, authCtx.Okta)
	assert.Equal(t, "https://company.okta.com", authCtx.Okta.OrgURL)
	assert.Equal(t, "test-access-token", authCtx.Okta.AccessToken)
	assert.Equal(t, "test-id-token", authCtx.Okta.IDToken)
	assert.Contains(t, authCtx.Okta.TokensFile, "test-provider")
	assert.Contains(t, authCtx.Okta.TokensFile, "tokens.json")
	assert.Contains(t, authCtx.Okta.ConfigDir, "test-provider")
}

func TestSetEnvironmentVariables_NilAuthContext(t *testing.T) {
	stackInfo := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(nil, stackInfo)
	require.NoError(t, err) // Graceful no-op.
}

func TestSetEnvironmentVariables_NilOkta(t *testing.T) {
	authCtx := &schema.AuthContext{Okta: nil}
	stackInfo := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authCtx, stackInfo)
	require.NoError(t, err) // Graceful no-op.
}

func TestSetEnvironmentVariables_NilStackInfo(t *testing.T) {
	authCtx := &schema.AuthContext{
		Okta: &schema.OktaAuthContext{
			OrgURL:      "https://company.okta.com",
			AccessToken: "test-token",
		},
	}
	err := SetEnvironmentVariables(authCtx, nil)
	require.NoError(t, err) // Graceful no-op.
}

func TestSetEnvironmentVariables_SetsVars(t *testing.T) {
	authCtx := &schema.AuthContext{
		Okta: &schema.OktaAuthContext{
			OrgURL:      "https://company.okta.com",
			AccessToken: "test-access-token",
			ConfigDir:   "/path/to/config",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: map[string]any{
			"EXISTING_VAR": "existing-value",
		},
	}

	err := SetEnvironmentVariables(authCtx, stackInfo)
	require.NoError(t, err)

	assert.Equal(t, "https://company.okta.com", stackInfo.ComponentEnvSection["OKTA_ORG_URL"])
	assert.Equal(t, "https://company.okta.com", stackInfo.ComponentEnvSection["OKTA_BASE_URL"])
	assert.Equal(t, "test-access-token", stackInfo.ComponentEnvSection["OKTA_OAUTH2_ACCESS_TOKEN"])
	assert.Equal(t, "/path/to/config", stackInfo.ComponentEnvSection["OKTA_CONFIG_DIR"])
	assert.Equal(t, "existing-value", stackInfo.ComponentEnvSection["EXISTING_VAR"])
}

func TestSetEnvironmentVariables_ClearsConflicting(t *testing.T) {
	authCtx := &schema.AuthContext{
		Okta: &schema.OktaAuthContext{
			OrgURL:      "https://company.okta.com",
			AccessToken: "new-token",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: map[string]any{
			"OKTA_API_TOKEN":    "old-api-token",
			"OKTA_CLIENT_ID":    "old-client-id",
			"OKTA_PRIVATE_KEY":  "old-private-key",
			"SAFE_VAR":          "safe-value",
		},
	}

	err := SetEnvironmentVariables(authCtx, stackInfo)
	require.NoError(t, err)

	// Conflicting vars should be cleared.
	_, hasAPIToken := stackInfo.ComponentEnvSection["OKTA_API_TOKEN"]
	_, hasClientID := stackInfo.ComponentEnvSection["OKTA_CLIENT_ID"]
	_, hasPrivKey := stackInfo.ComponentEnvSection["OKTA_PRIVATE_KEY"]

	assert.False(t, hasAPIToken)
	assert.False(t, hasClientID)
	assert.False(t, hasPrivKey)

	// Safe var preserved.
	assert.Equal(t, "safe-value", stackInfo.ComponentEnvSection["SAFE_VAR"])

	// New token set.
	assert.Equal(t, "new-token", stackInfo.ComponentEnvSection["OKTA_OAUTH2_ACCESS_TOKEN"])
}

func TestSetEnvironmentVariables_EmptyStackEnvSection(t *testing.T) {
	authCtx := &schema.AuthContext{
		Okta: &schema.OktaAuthContext{
			OrgURL:      "https://company.okta.com",
			AccessToken: "test-token",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: nil,
	}

	err := SetEnvironmentVariables(authCtx, stackInfo)
	require.NoError(t, err)

	assert.NotNil(t, stackInfo.ComponentEnvSection)
	assert.Equal(t, "https://company.okta.com", stackInfo.ComponentEnvSection["OKTA_ORG_URL"])
}

func TestSetEnvironmentVariables_NonStringEnvValues(t *testing.T) {
	authCtx := &schema.AuthContext{
		Okta: &schema.OktaAuthContext{
			OrgURL:      "https://company.okta.com",
			AccessToken: "test-token",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		ComponentEnvSection: map[string]any{
			"STRING_VAR":  "string-value",
			"INT_VAR":     42,
			"BOOL_VAR":    true,
		},
	}

	err := SetEnvironmentVariables(authCtx, stackInfo)
	require.NoError(t, err)

	// Only string values should be preserved from the original env section.
	assert.Equal(t, "string-value", stackInfo.ComponentEnvSection["STRING_VAR"])
	// Non-string values are dropped during the map[string]any -> map[string]string conversion.
	_, hasIntVar := stackInfo.ComponentEnvSection["INT_VAR"]
	assert.False(t, hasIntVar)
}

// mockCredentials is a simple mock that implements ICredentials for testing non-Okta cases.
type mockCredentials struct{}

func (m *mockCredentials) IsExpired() bool                                            { return false }
func (m *mockCredentials) GetExpiration() (*time.Time, error)                         { return nil, nil }
func (m *mockCredentials) BuildWhoamiInfo(_ *types.WhoamiInfo)                        {}
func (m *mockCredentials) Validate(_ context.Context) (*types.ValidationInfo, error)  { return nil, nil }
