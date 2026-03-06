package aws

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGeneratePKCEPair(t *testing.T) {
	verifier, challenge, err := generatePKCEPair()
	require.NoError(t, err)

	// Verifier should be base64url-encoded 32 bytes (43 characters without padding).
	assert.Len(t, verifier, 43)

	// Challenge should be base64url-encoded SHA-256 hash (43 characters without padding).
	assert.Len(t, challenge, 43)

	// Verify challenge is SHA-256 of verifier.
	hash := sha256.Sum256([]byte(verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	assert.Equal(t, expectedChallenge, challenge)

	// Verify no padding characters.
	assert.NotContains(t, verifier, "=")
	assert.NotContains(t, challenge, "=")
}

func TestGeneratePKCEPair_Uniqueness(t *testing.T) {
	verifier1, _, err := generatePKCEPair()
	require.NoError(t, err)

	verifier2, _, err := generatePKCEPair()
	require.NoError(t, err)

	// Two calls should produce different verifiers.
	assert.NotEqual(t, verifier1, verifier2)
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(16)
	require.NoError(t, err)
	assert.NotEmpty(t, s1)

	s2, err := generateRandomString(16)
	require.NoError(t, err)

	// Should be unique.
	assert.NotEqual(t, s1, s2)
}

func TestGetSigninEndpoint(t *testing.T) {
	tests := []struct {
		region   string
		expected string
	}{
		{"us-east-1", "https://signin.aws.amazon.com"},
		{"us-east-2", "https://us-east-2.signin.aws.amazon.com"},
		{"us-west-2", "https://us-west-2.signin.aws.amazon.com"},
		{"eu-west-1", "https://eu-west-1.signin.aws.amazon.com"},
		{"ap-southeast-1", "https://ap-southeast-1.signin.aws.amazon.com"},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			result := getSigninEndpoint(tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildAuthorizationURL(t *testing.T) {
	authURL := buildAuthorizationURL("us-east-2", 12345, "test-challenge", "test-state")

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)

	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "us-east-2.signin.aws.amazon.com", parsed.Host)
	assert.Equal(t, "/v1/authorize", parsed.Path)

	params := parsed.Query()
	assert.Equal(t, webflowOAuthClientID, params.Get("client_id"))
	assert.Equal(t, "http://127.0.0.1:12345/oauth/callback", params.Get("redirect_uri"))
	assert.Equal(t, "code", params.Get("response_type"))
	assert.Equal(t, "test-challenge", params.Get("code_challenge"))
	assert.Equal(t, webflowCodeChallengeMethod, params.Get("code_challenge_method"))
	assert.Equal(t, "openid", params.Get("scope"))
	assert.Equal(t, "test-state", params.Get("state"))
}

func TestBuildAuthorizationURL_UsEast1(t *testing.T) {
	authURL := buildAuthorizationURL("us-east-1", 8080, "ch", "st")

	parsed, err := url.Parse(authURL)
	require.NoError(t, err)

	// us-east-1 uses global endpoint without region prefix.
	assert.Equal(t, "signin.aws.amazon.com", parsed.Host)
}

func TestStartCallbackServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := "test-state-123"
	listener, resultCh, err := startCallbackServer(ctx, state)
	require.NoError(t, err)
	require.NotNil(t, listener)

	addr := listener.Addr().String()

	// Simulate a successful callback.
	callbackURL := fmt.Sprintf("http://%s%s?code=test-code-456&state=%s", addr, webflowCallbackPath, state)
	resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read the result.
	result := <-resultCh
	assert.NoError(t, result.err)
	assert.Equal(t, "test-code-456", result.code)
	assert.Equal(t, state, result.state)
}

func TestStartCallbackServer_MissingCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := "test-state"
	listener, resultCh, err := startCallbackServer(ctx, state)
	require.NoError(t, err)

	addr := listener.Addr().String()

	// Callback without code parameter.
	callbackURL := fmt.Sprintf("http://%s%s?state=%s", addr, webflowCallbackPath, state)
	resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-resultCh
	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "missing authorization code")
}

func TestStartCallbackServer_StateMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	state := "expected-state"
	listener, resultCh, err := startCallbackServer(ctx, state)
	require.NoError(t, err)

	addr := listener.Addr().String()

	// Callback with wrong state.
	callbackURL := fmt.Sprintf("http://%s%s?code=test-code&state=wrong-state", addr, webflowCallbackPath)
	resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-resultCh
	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "state mismatch")
}

func TestStartCallbackServer_AuthError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, resultCh, err := startCallbackServer(ctx, "state")
	require.NoError(t, err)

	addr := listener.Addr().String()

	// Callback with error from authorization server.
	callbackURL := fmt.Sprintf("http://%s%s?error=access_denied&error_description=User+denied+access", addr, webflowCallbackPath)
	resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-resultCh
	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "access_denied")
}

func TestExchangeCodeForCredentials_Success(t *testing.T) {
	// Mock token endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKIAIOSFODNN7EXAMPLE",
				"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken":    "FwoGZXIvYXdzEBYaDH...",
			},
			"expiresIn":    900,
			"refreshToken": "refresh-token-value",
			"tokenType":    "urn:aws:params:oauth:token-type:access_token_sigv4",
		})
	}))
	defer server.Close()

	// Override the endpoint for this test by using the mock server as the HTTP client.
	// We need a custom approach since the endpoint is constructed from the region.
	// Instead, we'll test callTokenEndpoint directly with a mock client.
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Forward to test server.
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	resp, err := exchangeCodeForCredentials(
		context.Background(),
		mockClient,
		"us-east-2",
		"auth-code-123",
		"code-verifier-abc",
		"http://127.0.0.1:8080/oauth/callback",
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", resp.AccessToken.AccessKeyID)
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", resp.AccessToken.SecretAccessKey)
	assert.Equal(t, "FwoGZXIvYXdzEBYaDH...", resp.AccessToken.SessionToken)
	assert.Equal(t, 900, resp.ExpiresIn)
	assert.Equal(t, "refresh-token-value", resp.RefreshToken)
}

func TestExchangeCodeForCredentials_HTTPError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	resp, err := exchangeCodeForCredentials(
		context.Background(),
		mockClient,
		"us-east-2",
		"code",
		"verifier",
		"http://127.0.0.1:8080/oauth/callback",
	)

	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestExchangeCodeForCredentials_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "The authorization code has expired",
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	resp, err := exchangeCodeForCredentials(
		context.Background(),
		mockClient,
		"us-east-2",
		"expired-code",
		"verifier",
		"http://127.0.0.1:8080/oauth/callback",
	)

	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestExchangeCodeForCredentials_EmptyCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "",
				"secretAccessKey": "",
				"sessionToken":    "",
			},
			"expiresIn": 900,
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	resp, err := exchangeCodeForCredentials(
		context.Background(),
		mockClient,
		"us-east-2",
		"code",
		"verifier",
		"http://127.0.0.1:8080/oauth/callback",
	)

	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing credentials")
}

func TestTokenResponseToCredentials(t *testing.T) {
	resp := &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
		},
		ExpiresIn:    900,
		RefreshToken: "refresh",
	}

	creds := tokenResponseToCredentials(resp, "us-west-2")

	assert.Equal(t, "AKID", creds.AccessKeyID)
	assert.Equal(t, "SECRET", creds.SecretAccessKey)
	assert.Equal(t, "TOKEN", creds.SessionToken)
	assert.Equal(t, "us-west-2", creds.Region)
	assert.NotEmpty(t, creds.Expiration)

	// Verify expiration is approximately 15 minutes from now.
	expTime, err := time.Parse(time.RFC3339, creds.Expiration)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(900*time.Second), expTime, 5*time.Second)
}

func TestTokenResponseToCredentials_NoExpiry(t *testing.T) {
	resp := &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
		},
		ExpiresIn: 0,
	}

	creds := tokenResponseToCredentials(resp, "us-east-1")
	assert.Empty(t, creds.Expiration)
}

func TestIsWebflowEnabled(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]interface{}
		expected    bool
	}{
		{
			name:        "nil credentials - default enabled",
			credentials: nil,
			expected:    true,
		},
		{
			name:        "empty credentials - default enabled",
			credentials: map[string]interface{}{},
			expected:    true,
		},
		{
			name: "webflow_enabled not set - default enabled",
			credentials: map[string]interface{}{
				"access_key_id": "test",
			},
			expected: true,
		},
		{
			name: "webflow_enabled explicit true",
			credentials: map[string]interface{}{
				"webflow_enabled": true,
			},
			expected: true,
		},
		{
			name: "webflow_enabled explicit false",
			credentials: map[string]interface{}{
				"webflow_enabled": false,
			},
			expected: false,
		},
		{
			name: "webflow_enabled wrong type - default enabled",
			credentials: map[string]interface{}{
				"webflow_enabled": "yes",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := &userIdentity{
				name: "test",
				config: &schema.Identity{
					Kind:        "aws/user",
					Credentials: tt.credentials,
				},
			}
			assert.Equal(t, tt.expected, identity.isWebflowEnabled())
		})
	}
}

func TestResolveCredentialsViaWebflow_Disabled(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"webflow_enabled": false,
			},
		},
	}

	creds, err := identity.resolveCredentialsViaWebflow(context.Background())
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowDisabled)
}

func TestRefreshCache_SaveAndLoad(t *testing.T) {
	// Use a temp directory for cache.
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test-user",
		realm: "test-realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	expiresAt := time.Now().Add(12 * time.Hour).Truncate(time.Second)

	// Save refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "my-refresh-token",
		Region:       "us-east-2",
		ExpiresAt:    expiresAt,
	})

	// Load refresh cache.
	cache, err := identity.loadRefreshCache()
	require.NoError(t, err)
	assert.Equal(t, "my-refresh-token", cache.RefreshToken)
	assert.Equal(t, "us-east-2", cache.Region)
	// Time comparison with tolerance for JSON serialization.
	assert.WithinDuration(t, expiresAt, cache.ExpiresAt, time.Second)

	// Verify file path uses filepath.Join.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	assert.Contains(t, cachePath, filepath.Join("aws-webflow", "test-user-test-realm", "refresh.json"))
}

func TestRefreshCache_LoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "nonexistent",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	cache, err := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, err)
}

func TestRefreshCache_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save then delete.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "token",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	identity.deleteRefreshCache()

	// Should not be loadable after delete.
	cache, err := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, err)
}

func TestRefreshWebflowCredentials_Success(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save a valid refresh token.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "valid-refresh-token",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(11 * time.Hour),
	})

	// Mock the token endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "REFRESHED_AKID",
				"secretAccessKey": "REFRESHED_SECRET",
				"sessionToken":    "REFRESHED_TOKEN",
			},
			"expiresIn":    900,
			"refreshToken": "new-refresh-token",
		})
	}))
	defer server.Close()

	// Override the default HTTP client.
	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "REFRESHED_AKID", creds.AccessKeyID)
	assert.Equal(t, "REFRESHED_SECRET", creds.SecretAccessKey)
	assert.Equal(t, "REFRESHED_TOKEN", creds.SessionToken)
}

func TestRefreshWebflowCredentials_ExpiredSession(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save an expired refresh token.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "expired-refresh-token",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Already expired.
	})

	creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)
}

func TestRefreshWebflowCredentials_NoCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)
}

func TestCallbackServer_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, _, err := startCallbackServer(ctx, "state")
	require.NoError(t, err)

	addr := listener.Addr().String()

	// Cancel the context to trigger server shutdown.
	cancel()

	// Give the server a moment to shut down.
	time.Sleep(100 * time.Millisecond)

	// Connection should fail after shutdown.
	_, err = http.Get(fmt.Sprintf("http://%s%s", addr, webflowCallbackPath)) //nolint:gosec // Test code.
	assert.Error(t, err)
}

func TestExchangeRefreshToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, webflowGrantTypeRefresh, body["grantType"])
		assert.Equal(t, "my-refresh-token", body["refreshToken"])

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "NEW_AKID",
				"secretAccessKey": "NEW_SECRET",
				"sessionToken":    "NEW_TOKEN",
			},
			"expiresIn":    900,
			"refreshToken": "updated-refresh-token",
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	resp, err := exchangeRefreshToken(context.Background(), mockClient, "us-east-2", "my-refresh-token")
	require.NoError(t, err)
	assert.Equal(t, "NEW_AKID", resp.AccessToken.AccessKeyID)
	assert.Equal(t, "updated-refresh-token", resp.RefreshToken)
}

func TestWebflowIntegration_AuthenticateFallback(t *testing.T) {
	// Test that Authenticate() tries webflow when no long-lived credentials exist.
	// We can't easily test the full browser flow, but we can verify the webflow path
	// is invoked (and returns disabled when webflow_enabled is false).
	t.Setenv("GO_TEST", "1")

	identity := &userIdentity{
		name: "no-creds-user",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"webflow_enabled": false,
			},
		},
		realm: "test",
	}

	// Disable credential prompting for this test.
	origPrompt := PromptCredentialsFunc
	PromptCredentialsFunc = nil
	defer func() { PromptCredentialsFunc = origPrompt }()

	ctx := types.WithAllowPrompts(context.Background(), false)
	creds, err := identity.Authenticate(ctx, nil)

	// Should fail because: no YAML creds, no keyring creds, webflow disabled.
	assert.Nil(t, creds)
	assert.Error(t, err)
}

func TestCacheFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "perm-test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "token",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)

	info, err := os.Stat(cachePath)
	require.NoError(t, err)
	// File should be readable/writable only by owner.
	assert.Equal(t, os.FileMode(webflowCacheFilePerms), info.Mode().Perm())
}

// mockHTTPClient is a test helper for mocking HTTP requests.
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}
