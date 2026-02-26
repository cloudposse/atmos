package okta

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- extractDeviceCodeConfig tests ---

func TestExtractDeviceCodeConfig_NilSpec(t *testing.T) {
	cfg := extractDeviceCodeConfig(nil)
	assert.Equal(t, defaultAuthorizationServer, cfg.AuthorizationServer)
	assert.Empty(t, cfg.OrgURL)
	assert.Empty(t, cfg.ClientID)
	assert.Empty(t, cfg.Scopes)
	assert.Empty(t, cfg.BasePath)
}

func TestExtractDeviceCodeConfig_FullSpec(t *testing.T) {
	spec := map[string]any{
		"org_url":              "https://company.okta.com",
		"client_id":            "test-client-id",
		"authorization_server": "custom-server",
		"scopes":               "openid profile email",
		"files": map[string]any{
			"base_path": "/custom/path",
		},
	}

	cfg := extractDeviceCodeConfig(spec)
	assert.Equal(t, "https://company.okta.com", cfg.OrgURL)
	assert.Equal(t, "test-client-id", cfg.ClientID)
	assert.Equal(t, "custom-server", cfg.AuthorizationServer)
	assert.Equal(t, []string{"openid", "profile", "email"}, cfg.Scopes)
	assert.Equal(t, "/custom/path", cfg.BasePath)
}

func TestExtractDeviceCodeConfig_ScopesAsArray(t *testing.T) {
	spec := map[string]any{
		"org_url":   "https://company.okta.com",
		"client_id": "test-client-id",
		"scopes":    []any{"openid", "profile", "offline_access"},
	}

	cfg := extractDeviceCodeConfig(spec)
	assert.Equal(t, []string{"openid", "profile", "offline_access"}, cfg.Scopes)
}

func TestExtractDeviceCodeConfig_EmptyAuthServer(t *testing.T) {
	spec := map[string]any{
		"authorization_server": "",
	}

	cfg := extractDeviceCodeConfig(spec)
	assert.Equal(t, defaultAuthorizationServer, cfg.AuthorizationServer)
}

func TestExtractDeviceCodeConfig_MixedScopeTypes(t *testing.T) {
	// Non-string scopes in array are skipped.
	spec := map[string]any{
		"scopes": []any{"openid", 42, "profile"},
	}

	cfg := extractDeviceCodeConfig(spec)
	assert.Equal(t, []string{"openid", "profile"}, cfg.Scopes)
}

// --- NewDeviceCodeProvider tests ---

func TestNewDeviceCodeProvider_NilConfig(t *testing.T) {
	_, err := NewDeviceCodeProvider("test", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestNewDeviceCodeProvider_WrongKind(t *testing.T) {
	config := &schema.Provider{Kind: "aws/sso"}
	_, err := NewDeviceCodeProvider("test", config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderKind)
}

func TestNewDeviceCodeProvider_MissingOrgURL(t *testing.T) {
	config := &schema.Provider{
		Kind: "okta/device-code",
		Spec: map[string]any{
			"client_id": "test-client-id",
		},
	}
	_, err := NewDeviceCodeProvider("test", config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
	assert.Contains(t, err.Error(), "org_url")
}

func TestNewDeviceCodeProvider_MissingClientID(t *testing.T) {
	config := &schema.Provider{
		Kind: "okta/device-code",
		Spec: map[string]any{
			"org_url": "https://company.okta.com",
		},
	}
	_, err := NewDeviceCodeProvider("test", config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
	assert.Contains(t, err.Error(), "client_id")
}

func TestNewDeviceCodeProvider_Valid(t *testing.T) {
	config := &schema.Provider{
		Kind: "okta/device-code",
		Spec: map[string]any{
			"org_url":   "https://company.okta.com",
			"client_id": "test-client-id",
		},
	}
	p, err := NewDeviceCodeProvider("test-provider", config)
	require.NoError(t, err)

	assert.Equal(t, "okta/device-code", p.Kind())
	assert.Equal(t, "test-provider", p.Name())
	assert.Equal(t, "https://company.okta.com", p.orgURL)
	assert.Equal(t, "test-client-id", p.clientID)
	assert.Equal(t, defaultAuthorizationServer, p.authorizationServer)
	// Default scopes.
	assert.Equal(t, []string{"openid", "profile", "offline_access"}, p.scopes)
}

func TestNewDeviceCodeProvider_CustomScopes(t *testing.T) {
	config := &schema.Provider{
		Kind: "okta/device-code",
		Spec: map[string]any{
			"org_url":   "https://company.okta.com",
			"client_id": "test-client-id",
			"scopes":    []any{"openid", "okta.users.read"},
		},
	}
	p, err := NewDeviceCodeProvider("test", config)
	require.NoError(t, err)
	assert.Equal(t, []string{"openid", "okta.users.read"}, p.scopes)
}

func TestNewDeviceCodeProvider_CustomBasePath(t *testing.T) {
	config := &schema.Provider{
		Kind: "okta/device-code",
		Spec: map[string]any{
			"org_url":   "https://company.okta.com",
			"client_id": "test-client-id",
			"files": map[string]any{
				"base_path": "/custom/path",
			},
		},
	}
	p, err := NewDeviceCodeProvider("test", config)
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", p.basePath)
}

// --- Provider interface methods ---

func TestDeviceCodeProvider_Kind(t *testing.T) {
	p := &deviceCodeProvider{}
	assert.Equal(t, "okta/device-code", p.Kind())
}

func TestDeviceCodeProvider_Name(t *testing.T) {
	p := &deviceCodeProvider{name: "my-provider"}
	assert.Equal(t, "my-provider", p.Name())
}

func TestDeviceCodeProvider_SetRealm(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	p := &deviceCodeProvider{
		name:        "test",
		fileManager: fm,
	}

	// SetRealm should clear the cached file manager.
	p.SetRealm("new-realm")
	assert.Equal(t, "new-realm", p.realm)
	assert.Nil(t, p.fileManager)
}

func TestDeviceCodeProvider_PreAuthenticate(t *testing.T) {
	p := &deviceCodeProvider{}
	err := p.PreAuthenticate(nil)
	require.NoError(t, err) // No-op.
}

func TestDeviceCodeProvider_Validate_Valid(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL:   "https://company.okta.com",
		clientID: "test-client-id",
	}
	err := p.Validate()
	require.NoError(t, err)
}

func TestDeviceCodeProvider_Validate_MissingOrgURL(t *testing.T) {
	p := &deviceCodeProvider{
		clientID: "test-client-id",
	}
	err := p.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestDeviceCodeProvider_Validate_MissingClientID(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL: "https://company.okta.com",
	}
	err := p.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestDeviceCodeProvider_Environment(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL: "https://company.okta.com",
	}

	env, err := p.Environment()
	require.NoError(t, err)
	assert.Equal(t, "https://company.okta.com", env["OKTA_ORG_URL"])
	assert.Equal(t, "https://company.okta.com", env["OKTA_BASE_URL"])
}

func TestDeviceCodeProvider_Environment_EmptyOrgURL(t *testing.T) {
	p := &deviceCodeProvider{}

	env, err := p.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestDeviceCodeProvider_Endpoints(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL:              "https://company.okta.com",
		authorizationServer: "default",
	}

	assert.Equal(t, "https://company.okta.com/oauth2/default/v1/device/authorize", p.getDeviceAuthorizationEndpoint())
	assert.Equal(t, "https://company.okta.com/oauth2/default/v1/token", p.getTokenEndpoint())
}

func TestDeviceCodeProvider_Endpoints_CustomServer(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL:              "https://company.okta.com",
		authorizationServer: "custom-auth",
	}

	assert.Equal(t, "https://company.okta.com/oauth2/custom-auth/v1/device/authorize", p.getDeviceAuthorizationEndpoint())
	assert.Equal(t, "https://company.okta.com/oauth2/custom-auth/v1/token", p.getTokenEndpoint())
}

func TestDeviceCodeProvider_PrepareEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		orgURL:   "https://company.okta.com",
		basePath: tempDir,
	}

	env := map[string]string{"EXISTING": "value"}
	result, err := p.PrepareEnvironment(context.Background(), env)
	require.NoError(t, err)

	assert.Equal(t, "https://company.okta.com", result["OKTA_ORG_URL"])
	assert.Equal(t, "https://company.okta.com", result["OKTA_BASE_URL"])
	assert.Contains(t, result["OKTA_CONFIG_DIR"], "test-provider")
	assert.Equal(t, "value", result["EXISTING"])
}

func TestDeviceCodeProvider_Paths(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	paths, err := p.Paths()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Contains(t, paths[0].Location, "test-provider")
	assert.Contains(t, paths[0].Location, "tokens.json")
	assert.True(t, paths[0].Required)
}

func TestDeviceCodeProvider_GetFilesDisplayPath(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	displayPath := p.GetFilesDisplayPath()
	assert.Contains(t, displayPath, "test-provider")
}

func TestDeviceCodeProvider_Logout(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write some tokens first.
	tokens := &oktaCloud.OktaTokens{
		AccessToken: "test-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	err = fm.WriteTokens("test-provider", tokens)
	require.NoError(t, err)
	assert.True(t, fm.TokensExist("test-provider"))

	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	err = p.Logout(context.Background())
	require.NoError(t, err)

	assert.False(t, fm.TokensExist("test-provider"))
}

func TestDeviceCodeProvider_Logout_NoTokens(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	err := p.Logout(context.Background())
	require.NoError(t, err) // Graceful no-op.
}

// --- startDeviceAuthorization tests ---

func TestStartDeviceAuthorization_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "test-client-id", r.FormValue("client_id"))
		assert.Equal(t, "openid profile", r.FormValue("scope"))

		resp := oktaCloud.DeviceAuthorizationResponse{
			DeviceCode:      "test-device-code-12345678",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://company.okta.com/activate",
			ExpiresIn:       600,
			Interval:        5,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Parse server URL to extract host.
	serverURL, _ := url.Parse(server.URL)

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid", "profile"},
		authorizationServer: "oauth2/default/v1/device", // Hack: build full path.
		httpClient:          server.Client(),
	}
	// Override the endpoint to use the test server.
	p.authorizationServer = "default"

	// We need the provider to hit the test server, so override orgURL.
	_ = serverURL

	ctx := context.Background()
	deviceAuth, err := p.startDeviceAuthorization(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-device-code-12345678", deviceAuth.DeviceCode)
	assert.Equal(t, "ABCD-EFGH", deviceAuth.UserCode)
	assert.Equal(t, "https://company.okta.com/activate", deviceAuth.VerificationURI)
	assert.Equal(t, 600, deviceAuth.ExpiresIn)
	assert.Equal(t, 5, deviceAuth.Interval)
}

func TestStartDeviceAuthorization_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_client","error_description":"Unknown client"}`))
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "bad-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	ctx := context.Background()
	_, err := p.startDeviceAuthorization(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "400")
}

func TestStartDeviceAuthorization_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	ctx := context.Background()
	_, err := p.startDeviceAuthorization(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestStartDeviceAuthorization_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second) // Slow response.
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          &http.Client{Timeout: 100 * time.Millisecond},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := p.startDeviceAuthorization(ctx)
	require.Error(t, err)
}

// --- tryGetToken tests ---

func TestTryGetToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := oktaCloud.TokenResponse{
			AccessToken:  "new-access-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "new-refresh-token",
			IDToken:      "new-id-token",
			Scope:        "openid profile",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")
	data.Set("device_code", "test-device-code")
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	tokens, shouldContinue, err := p.tryGetToken(context.Background(), data)
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.False(t, shouldContinue)
	assert.Equal(t, "new-access-token", tokens.AccessToken)
	assert.Equal(t, "new-refresh-token", tokens.RefreshToken)
	assert.Equal(t, "new-id-token", tokens.IDToken)
}

func TestTryGetToken_AuthorizationPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "authorization_pending",
			ErrorDescription: "The user has not yet completed authentication",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	tokens, shouldContinue, err := p.tryGetToken(context.Background(), data)
	require.NoError(t, err)
	assert.Nil(t, tokens)
	assert.True(t, shouldContinue)
}

func TestTryGetToken_SlowDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "slow_down",
			ErrorDescription: "Polling too frequently",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	tokens, shouldContinue, err := p.tryGetToken(context.Background(), data)
	require.NoError(t, err)
	assert.Nil(t, tokens)
	assert.True(t, shouldContinue)
}

func TestTryGetToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "access_denied",
			ErrorDescription: "The user denied the request",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	tokens, shouldContinue, err := p.tryGetToken(context.Background(), data)
	require.NoError(t, err)
	assert.Nil(t, tokens)
	assert.False(t, shouldContinue)
}

func TestTryGetToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "expired_token",
			ErrorDescription: "The device code has expired",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	_, _, err := p.tryGetToken(context.Background(), data)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrOktaDeviceCodeExpired)
}

func TestTryGetToken_UnknownError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "server_error",
			ErrorDescription: "Internal server error",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	_, _, err := p.tryGetToken(context.Background(), data)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "server_error")
}

func TestTryGetToken_InvalidErrorJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	data := url.Values{}
	data.Set("client_id", "test-client-id")

	_, _, err := p.tryGetToken(context.Background(), data)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// --- refreshToken tests ---

func TestRefreshToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "test-client-id", r.FormValue("client_id"))
		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		assert.Equal(t, "old-refresh-token", r.FormValue("refresh_token"))
		assert.Equal(t, "openid profile", r.FormValue("scope"))

		resp := oktaCloud.TokenResponse{
			AccessToken:  "refreshed-access-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "new-refresh-token",
			IDToken:      "refreshed-id-token",
			Scope:        "openid profile",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid", "profile"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	tokens, err := p.refreshToken(context.Background(), "old-refresh-token")
	require.NoError(t, err)
	assert.Equal(t, "refreshed-access-token", tokens.AccessToken)
	assert.Equal(t, "new-refresh-token", tokens.RefreshToken)
	assert.Equal(t, "refreshed-id-token", tokens.IDToken)
}

func TestRefreshToken_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Refresh token is expired",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	_, err := p.refreshToken(context.Background(), "expired-refresh-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrOktaTokenRefreshFailed)
	assert.Contains(t, err.Error(), "invalid_grant")
}

func TestRefreshToken_NonJSONErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	_, err := p.refreshToken(context.Background(), "some-refresh-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrOktaTokenRefreshFailed)
	assert.Contains(t, err.Error(), "500")
}

func TestRefreshToken_InvalidSuccessJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	_, err := p.refreshToken(context.Background(), "some-refresh-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrOktaTokenRefreshFailed)
}

// --- tryCachedTokens tests ---

func TestTryCachedTokens_NoTokensFile(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	tokens, err := p.tryCachedTokens(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tokens)
}

func TestTryCachedTokens_ValidTokens(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	cachedTokens := &oktaCloud.OktaTokens{
		AccessToken: "cached-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(time.Hour),
		IDToken:     "cached-id-token",
	}
	err = fm.WriteTokens("test-provider", cachedTokens)
	require.NoError(t, err)

	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	tokens, err := p.tryCachedTokens(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Equal(t, "cached-token", tokens.AccessToken)
}

func TestTryCachedTokens_ExpiredWithRefresh(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write expired tokens with valid refresh token.
	cachedTokens := &oktaCloud.OktaTokens{
		AccessToken:           "expired-token",
		TokenType:             "Bearer",
		ExpiresAt:             time.Now().Add(-time.Hour),
		RefreshToken:          "valid-refresh-token",
		RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	err = fm.WriteTokens("test-provider", cachedTokens)
	require.NoError(t, err)

	// Mock refresh endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := oktaCloud.TokenResponse{
			AccessToken:  "refreshed-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "new-refresh-token",
			Scope:        "openid profile",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		name:                "test-provider",
		basePath:            tempDir,
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid", "profile"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	tokens, err := p.tryCachedTokens(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Equal(t, "refreshed-token", tokens.AccessToken)
}

func TestTryCachedTokens_ExpiredNoRefresh(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write expired tokens without refresh token.
	cachedTokens := &oktaCloud.OktaTokens{
		AccessToken: "expired-token",
		TokenType:   "Bearer",
		ExpiresAt:   time.Now().Add(-time.Hour),
	}
	err = fm.WriteTokens("test-provider", cachedTokens)
	require.NoError(t, err)

	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	tokens, err := p.tryCachedTokens(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tokens)
}

func TestTryCachedTokens_ExpiredRefreshFails(t *testing.T) {
	tempDir := t.TempDir()
	fm, err := oktaCloud.NewOktaFileManager(tempDir, "")
	require.NoError(t, err)

	// Write expired tokens with refresh token.
	cachedTokens := &oktaCloud.OktaTokens{
		AccessToken:           "expired-token",
		TokenType:             "Bearer",
		ExpiresAt:             time.Now().Add(-time.Hour),
		RefreshToken:          "bad-refresh-token",
		RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	err = fm.WriteTokens("test-provider", cachedTokens)
	require.NoError(t, err)

	// Mock refresh endpoint that fails.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{
			Error:            "invalid_grant",
			ErrorDescription: "Refresh token revoked",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		name:                "test-provider",
		basePath:            tempDir,
		orgURL:              server.URL,
		clientID:            "test-client-id",
		scopes:              []string{"openid"},
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	// Returns nil (not error) — falls back to re-authentication.
	tokens, err := p.tryCachedTokens(context.Background())
	require.NoError(t, err)
	assert.Nil(t, tokens)
}

// --- tokensToCredentials tests ---

func TestTokensToCredentials(t *testing.T) {
	p := &deviceCodeProvider{
		orgURL: "https://company.okta.com",
	}

	tokens := &oktaCloud.OktaTokens{
		AccessToken:           "test-access-token",
		IDToken:               "test-id-token",
		RefreshToken:          "test-refresh-token",
		ExpiresAt:             time.Now().Add(time.Hour),
		RefreshTokenExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Scope:                 "openid profile",
	}

	creds, err := p.tokensToCredentials(tokens)
	require.NoError(t, err)
	assert.Equal(t, "https://company.okta.com", creds.OrgURL)
	assert.Equal(t, "test-access-token", creds.AccessToken)
	assert.Equal(t, "test-id-token", creds.IDToken)
	assert.Equal(t, "test-refresh-token", creds.RefreshToken)
	assert.Equal(t, "openid profile", creds.Scope)
}

// --- getFileManager tests ---

func TestGetFileManager_Caching(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	fm1, err := p.getFileManager()
	require.NoError(t, err)
	require.NotNil(t, fm1)

	fm2, err := p.getFileManager()
	require.NoError(t, err)
	// Same pointer (cached).
	assert.Same(t, fm1, fm2)
}

func TestGetFileManager_WithCustomBasePath(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
	}

	fm, err := p.getFileManager()
	require.NoError(t, err)
	assert.Equal(t, tempDir, fm.GetBaseDir())
}

// --- pollForToken tests ---

func TestPollForToken_ImmediateSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		resp := oktaCloud.TokenResponse{
			AccessToken: "polled-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
			Scope:       "openid",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	deviceAuth := &oktaCloud.DeviceAuthorizationResponse{
		DeviceCode: "test-device-code",
		ExpiresIn:  600,
		Interval:   1, // 1 second for fast test.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tokens, err := p.pollForToken(ctx, deviceAuth)
	require.NoError(t, err)
	assert.Equal(t, "polled-access-token", tokens.AccessToken)
	assert.Equal(t, 1, callCount)
}

func TestPollForToken_PendingThenSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusBadRequest)
			resp := oktaCloud.TokenErrorResponse{
				Error: "authorization_pending",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := oktaCloud.TokenResponse{
			AccessToken: "final-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	deviceAuth := &oktaCloud.DeviceAuthorizationResponse{
		DeviceCode: "test-device-code",
		ExpiresIn:  600,
		Interval:   1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := p.pollForToken(ctx, deviceAuth)
	require.NoError(t, err)
	assert.Equal(t, "final-access-token", tokens.AccessToken)
	assert.Equal(t, 3, callCount)
}

func TestPollForToken_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		resp := oktaCloud.TokenErrorResponse{Error: "authorization_pending"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	deviceAuth := &oktaCloud.DeviceAuthorizationResponse{
		DeviceCode: "test-device-code",
		ExpiresIn:  600,
		Interval:   1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := p.pollForToken(ctx, deviceAuth)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

func TestPollForToken_DefaultInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := oktaCloud.TokenResponse{
			AccessToken: "access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &deviceCodeProvider{
		orgURL:              server.URL,
		clientID:            "test-client-id",
		authorizationServer: "default",
		httpClient:          server.Client(),
	}

	deviceAuth := &oktaCloud.DeviceAuthorizationResponse{
		DeviceCode: "test-device-code",
		ExpiresIn:  600,
		Interval:   0, // Should default.
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := p.pollForToken(ctx, deviceAuth)
	require.NoError(t, err)
	assert.Equal(t, "access-token", tokens.AccessToken)
}

// --- Integration: file manager with realm ---

func TestDeviceCodeProvider_FileManagerWithRealm(t *testing.T) {
	tempDir := t.TempDir()
	p := &deviceCodeProvider{
		name:     "test-provider",
		basePath: tempDir,
		realm:    "test-realm",
	}

	fm, err := p.getFileManager()
	require.NoError(t, err)
	// When custom basePath is set, it's used directly (realm not appended).
	assert.Equal(t, tempDir, fm.GetBaseDir())

	// Provider dir should include the provider name.
	expectedProviderDir := filepath.Join(tempDir, "test-provider")
	assert.Equal(t, expectedProviderDir, fm.GetProviderDir("test-provider"))
}
