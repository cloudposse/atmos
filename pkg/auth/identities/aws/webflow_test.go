package aws

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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

	// Connection should fail after shutdown.
	require.Eventually(t, func() bool {
		_, err := http.Get(fmt.Sprintf("http://%s%s", addr, webflowCallbackPath)) //nolint:gosec // Test code.
		return err != nil
	}, 2*time.Second, 50*time.Millisecond, "server should shut down after context cancellation")
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
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix file permissions")
	}

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

func TestProcessTokenResponse_WithRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	tokenResp := &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
		},
		ExpiresIn:    900,
		RefreshToken: "my-refresh-token",
	}

	creds, err := identity.processTokenResponse(tokenResp, "us-west-2")
	require.NoError(t, err)
	assert.Equal(t, "AKID", creds.AccessKeyID)
	assert.Equal(t, "SECRET", creds.SecretAccessKey)
	assert.Equal(t, "TOKEN", creds.SessionToken)
	assert.Equal(t, "us-west-2", creds.Region)

	// Verify refresh token was cached.
	cache, cacheErr := identity.loadRefreshCache()
	require.NoError(t, cacheErr)
	assert.Equal(t, "my-refresh-token", cache.RefreshToken)
	assert.Equal(t, "us-west-2", cache.Region)
	assert.WithinDuration(t, time.Now().Add(webflowSessionDuration), cache.ExpiresAt, 5*time.Second)
}

func TestProcessTokenResponse_WithoutRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	tokenResp := &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "TOKEN",
		},
		ExpiresIn: 900,
		// No RefreshToken.
	}

	creds, err := identity.processTokenResponse(tokenResp, "eu-west-1")
	require.NoError(t, err)
	assert.Equal(t, "AKID", creds.AccessKeyID)
	assert.Equal(t, "eu-west-1", creds.Region)

	// No cache should exist.
	_, cacheErr := identity.loadRefreshCache()
	assert.Error(t, cacheErr)
}

func TestResolveCredentialsViaWebflow_RefreshSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-2",
			},
		},
	}

	// Save a valid refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "valid-token",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(10 * time.Hour),
	})

	// Mock token endpoint.
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

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	creds, err := identity.resolveCredentialsViaWebflow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "REFRESHED_AKID", creds.AccessKeyID)
	assert.Equal(t, "REFRESHED_SECRET", creds.SecretAccessKey)
	assert.Equal(t, "REFRESHED_TOKEN", creds.SessionToken)
	assert.Equal(t, "us-east-2", creds.Region)
}

func TestBrowserWebflow_NoPrompts(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind:        "aws/user",
			Credentials: map[string]interface{}{},
		},
	}

	// Context with prompts disallowed.
	ctx := types.WithAllowPrompts(context.Background(), false)

	creds, err := identity.browserWebflow(ctx, "us-east-1")
	assert.Nil(t, creds)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}

func TestWaitForCallbackSimple_Success(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Mock token server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_SIMPLE",
				"secretAccessKey": "SECRET_SIMPLE",
				"sessionToken":    "TOKEN_SIMPLE",
			},
			"expiresIn":    900,
			"refreshToken": "refresh",
		})
	}))
	defer server.Close()

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{code: "auth-code", state: "state-123"}

	resp, err := identity.waitForCallbackSimple(context.Background(), resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_SIMPLE", resp.AccessToken.AccessKeyID)
}

func TestWaitForCallbackSimple_Error(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{err: fmt.Errorf("callback failed")}

	resp, err := identity.waitForCallbackSimple(context.Background(), resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}

func TestWaitForCallbackSimple_ContextCancelled(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel.

	resultCh := make(chan webflowResult) // Unbuffered, will never receive.

	resp, err := identity.waitForCallbackSimple(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTimeout)
}

func TestCallTokenEndpoint_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json")) //nolint:errcheck // Test code.
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
	assert.Contains(t, err.Error(), "parse token response")
}

func TestCallTokenEndpoint_NonOK_NoErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error")) //nolint:errcheck // Test code.
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestCallTokenEndpoint_NonOK_WithErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Authorization code expired",
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "expired-code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestCallTokenEndpoint_HTTPClientError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("network unreachable")
		},
	}

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestCallTokenEndpoint_MissingCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "",
				"secretAccessKey": "",
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

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
	assert.Contains(t, err.Error(), "missing credentials")
}

func TestRefreshWebflowCredentials_ExchangeFailureClearsCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save a valid (non-expired) refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "valid-but-rejected-token",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(10 * time.Hour),
	})

	// Mock endpoint that rejects the refresh token.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "Refresh token is invalid",
		})
	}))
	defer server.Close()

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)

	// Cache should have been deleted.
	cache, cacheErr := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, cacheErr)
}

func TestRefreshWebflowCredentials_UpdatesRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	originalExpiresAt := time.Now().Add(10 * time.Hour).Truncate(time.Second)

	// Save initial refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "old-token",
		Region:       "us-east-2",
		ExpiresAt:    originalExpiresAt,
	})

	// Mock endpoint returns new refresh token.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID",
				"secretAccessKey": "SECRET",
				"sessionToken":    "TOKEN",
			},
			"expiresIn":    900,
			"refreshToken": "rotated-token",
		})
	}))
	defer server.Close()

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
	assert.Equal(t, "AKID", creds.AccessKeyID)

	// Verify cache was updated with new refresh token but same expiry.
	cache, cacheErr := identity.loadRefreshCache()
	require.NoError(t, cacheErr)
	assert.Equal(t, "rotated-token", cache.RefreshToken)
	// ExpiresAt should not change (session end time is fixed).
	assert.WithinDuration(t, originalExpiresAt, cache.ExpiresAt, time.Second)
}

func TestLoadRefreshCache_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Write invalid JSON to the cache file.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	err = os.WriteFile(cachePath, []byte("not-json"), webflowCacheFilePerms)
	require.NoError(t, err)

	cache, loadErr := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "parse refresh cache")
}

func TestLoadRefreshCache_EmptyRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Write cache with empty refresh token.
	cachePath, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	data, _ := json.Marshal(&webflowRefreshCache{
		RefreshToken: "",
		Region:       "us-east-1",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	err = os.WriteFile(cachePath, data, webflowCacheFilePerms)
	require.NoError(t, err)

	cache, loadErr := identity.loadRefreshCache()
	assert.Nil(t, cache)
	assert.Error(t, loadErr)
	assert.Contains(t, loadErr.Error(), "empty")
}

func TestWebflowSpinnerModel_View(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	tests := []struct {
		name     string
		model    webflowSpinnerModel
		contains string
	}{
		{
			name: "in progress",
			model: webflowSpinnerModel{
				spinner: s,
				message: "Waiting for auth",
				done:    false,
			},
			contains: "Waiting for auth",
		},
		{
			name: "done with error",
			model: webflowSpinnerModel{
				spinner: s,
				done:    true,
				result:  &webflowSpinnerTokenResult{err: fmt.Errorf("failed")},
			},
			contains: "Authentication failed",
		},
		{
			name: "done success",
			model: webflowSpinnerModel{
				spinner: s,
				done:    true,
				result:  &webflowSpinnerTokenResult{resp: &webflowTokenResponse{}},
			},
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := tt.model.View()
			if tt.contains != "" {
				assert.Contains(t, view, tt.contains)
			}
		})
	}
}

func TestWebflowSpinnerModel_Update_CtrlC(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	cancelled := false
	model := webflowSpinnerModel{
		spinner: s,
		message: "Waiting",
		tokenCh: make(<-chan webflowSpinnerTokenResult),
		cancel:  func() { cancelled = true },
	}

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	updated, _ := model.Update(msg)
	m := updated.(webflowSpinnerModel)

	assert.True(t, m.done)
	assert.NotNil(t, m.result)
	assert.ErrorIs(t, m.result.err, errUtils.ErrUserAborted)
	assert.True(t, cancelled)
}

func TestWebflowSpinnerModel_Update_TokenResult(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	cancelled := false
	model := webflowSpinnerModel{
		spinner: s,
		message: "Waiting",
		tokenCh: make(<-chan webflowSpinnerTokenResult),
		cancel:  func() { cancelled = true },
	}

	tokenResult := webflowSpinnerTokenResult{
		resp: &webflowTokenResponse{
			AccessToken: webflowAccessToken{
				AccessKeyID:     "AKID",
				SecretAccessKey: "SECRET",
				SessionToken:    "TOKEN",
			},
		},
	}

	updated, _ := model.Update(tokenResult)
	m := updated.(webflowSpinnerModel)

	assert.True(t, m.done)
	assert.NotNil(t, m.result)
	assert.NoError(t, m.result.err)
	assert.Equal(t, "AKID", m.result.resp.AccessToken.AccessKeyID)
	assert.True(t, cancelled)
}

func TestBrowserWebflowInteractive_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-2",
			},
		},
	}

	// Mock token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_INTERACTIVE",
				"secretAccessKey": "SECRET_INTERACTIVE",
				"sessionToken":    "TOKEN_INTERACTIVE",
			},
			"expiresIn":    900,
			"refreshToken": "refresh-interactive",
		})
	}))
	defer tokenServer.Close()

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(tokenServer.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	// Non-TTY so waitForCallbackWithSpinner takes the simple path (avoids bubbletea in test).
	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	// Mock display dialog to capture the auth URL and simulate callback.
	origDisplay := displayWebflowDialogFunc
	displayWebflowDialogFunc = func(authURL string) {
		parsed, _ := url.Parse(authURL)
		redirectURI := parsed.Query().Get("redirect_uri")
		state := parsed.Query().Get("state")
		go func() {
			time.Sleep(50 * time.Millisecond)
			callbackURL := fmt.Sprintf("%s?code=test-auth-code&state=%s", redirectURI, state)
			resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	defer func() { displayWebflowDialogFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.browserWebflowInteractive(ctx, "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "AKID_INTERACTIVE", creds.AccessKeyID)
	assert.Equal(t, "SECRET_INTERACTIVE", creds.SecretAccessKey)
	assert.Equal(t, "TOKEN_INTERACTIVE", creds.SessionToken)
}

func TestWaitForCallbackWithSpinner_NonTTY(t *testing.T) {
	identity := &userIdentity{
		name: "test",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Mock token endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_SPINNER",
				"secretAccessKey": "SECRET_SPINNER",
				"sessionToken":    "TOKEN_SPINNER",
			},
			"expiresIn": 900,
		})
	}))
	defer server.Close()

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	// Ensure non-TTY so it falls back to waitForCallbackSimple.
	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{code: "callback-code", state: "state"}

	resp, err := identity.waitForCallbackWithSpinner(context.Background(), resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	require.NoError(t, err)
	assert.Equal(t, "AKID_SPINNER", resp.AccessToken.AccessKeyID)
}

func TestBrowserWebflow_Interactive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-2",
			},
		},
	}

	// Mock token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_BW",
				"secretAccessKey": "SECRET_BW",
				"sessionToken":    "TOKEN_BW",
			},
			"expiresIn":    900,
			"refreshToken": "refresh-bw",
		})
	}))
	defer tokenServer.Close()

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(tokenServer.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	// Mock TTY: true for browserWebflow's dispatch to interactive, but non-TTY
	// for waitForCallbackWithSpinner to avoid bubbletea in tests.
	ttyCallCount := 0
	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool {
		ttyCallCount++
		// First call: browserWebflow checks TTY -> true (enters interactive path).
		// Second call: waitForCallbackWithSpinner checks TTY -> false (uses simple path).
		return ttyCallCount == 1
	}
	defer func() { webflowIsTTYFunc = origTTY }()

	// Mock display dialog to simulate callback (works regardless of IsCI).
	origDisplay := displayWebflowDialogFunc
	displayWebflowDialogFunc = func(authURL string) {
		parsed, _ := url.Parse(authURL)
		redirectURI := parsed.Query().Get("redirect_uri")
		state := parsed.Query().Get("state")
		go func() {
			time.Sleep(50 * time.Millisecond)
			callbackURL := fmt.Sprintf("%s?code=bw-code&state=%s", redirectURI, state)
			resp, err := http.Get(callbackURL) //nolint:gosec // Test code.
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	defer func() { displayWebflowDialogFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.browserWebflow(ctx, "us-east-2")
	require.NoError(t, err)
	assert.Equal(t, "AKID_BW", creds.AccessKeyID)
}

func TestDisplayWebflowDialog(t *testing.T) {
	// Just verify it doesn't panic.
	displayWebflowDialog("https://example.com/auth")
}

func TestDisplayWebflowDialogPlainText(t *testing.T) {
	// Just verify it doesn't panic.
	displayWebflowDialogPlainText("https://example.com/auth")
}

func TestWebflowIsTTY(t *testing.T) {
	// In test environment, stderr is typically not a TTY.
	result := webflowIsTTY()
	assert.False(t, result)
}

func TestWebflowIsInteractive(t *testing.T) {
	// Without force-tty, in test env this should return false.
	result := webflowIsInteractive()
	assert.False(t, result)
}

func TestWebflowSpinnerModel_Init(t *testing.T) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	tokenCh := make(chan webflowSpinnerTokenResult)

	model := webflowSpinnerModel{
		spinner: s,
		message: "Test",
		tokenCh: tokenCh,
		cancel:  func() {},
	}

	cmd := model.Init()
	assert.NotNil(t, cmd)
}

func TestWebflowSpinnerModel_CheckResult(t *testing.T) {
	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	tokenCh <- webflowSpinnerTokenResult{
		resp: &webflowTokenResponse{
			AccessToken: webflowAccessToken{AccessKeyID: "AKID_CHECK"},
		},
	}

	s := spinner.New()
	model := webflowSpinnerModel{
		spinner: s,
		tokenCh: tokenCh,
	}

	cmd := model.checkResult()
	require.NotNil(t, cmd)

	// Execute the command to get the result.
	msg := cmd()
	result, ok := msg.(webflowSpinnerTokenResult)
	require.True(t, ok)
	assert.Equal(t, "AKID_CHECK", result.resp.AccessToken.AccessKeyID)
}

func TestExchangeRefreshToken_HTTPError(t *testing.T) {
	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection reset")
		},
	}

	resp, err := exchangeRefreshToken(context.Background(), mockClient, "us-east-2", "token")
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

func TestDeleteRefreshCache_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Should not panic when deleting non-existent cache.
	identity.deleteRefreshCache()
}

func TestGetSigninEndpoint_Regions(t *testing.T) {
	tests := []struct {
		region   string
		expected string
	}{
		{"us-east-1", "https://signin.aws.amazon.com"},
		{"us-west-1", "https://us-west-1.signin.aws.amazon.com"},
		{"cn-north-1", "https://cn-north-1.signin.aws.amazon.com"},
		{"us-gov-west-1", "https://us-gov-west-1.signin.aws.amazon.com"},
	}

	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			assert.Equal(t, tt.expected, getSigninEndpoint(tt.region))
		})
	}
}

func TestCallTokenEndpoint_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_DIRECT",
				"secretAccessKey": "SECRET_DIRECT",
				"sessionToken":    "TOKEN_DIRECT",
			},
			"expiresIn":    900,
			"refreshToken": "refresh-direct",
			"tokenType":    "urn:aws:params:oauth:token-type:access_token_sigv4",
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := map[string]string{
		"clientId":  webflowOAuthClientID,
		"grantType": webflowGrantTypeAuthCode,
		"code":      "auth-code",
	}

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	require.NoError(t, err)
	assert.Equal(t, "AKID_DIRECT", resp.AccessToken.AccessKeyID)
	assert.Equal(t, "SECRET_DIRECT", resp.AccessToken.SecretAccessKey)
	assert.Equal(t, "TOKEN_DIRECT", resp.AccessToken.SessionToken)
	assert.Equal(t, 900, resp.ExpiresIn)
	assert.Equal(t, "refresh-direct", resp.RefreshToken)
}

func TestGetRefreshCachePath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "my-identity",
		realm: "my-realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	path, err := identity.getRefreshCachePath()
	require.NoError(t, err)
	assert.Contains(t, path, filepath.Join("aws-webflow", "my-identity-my-realm", "refresh.json"))
}

func TestIsTransientTokenError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error is not transient",
			err:      nil,
			expected: false,
		},
		{
			name:     "context.Canceled is transient",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "context.DeadlineExceeded is transient",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped context.Canceled is transient",
			err:      fmt.Errorf("token exchange: %w", context.Canceled),
			expected: true,
		},
		{
			name:     "wrapped context.DeadlineExceeded is transient",
			err:      fmt.Errorf("token exchange: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "net.Error is transient",
			err:      &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")},
			expected: true,
		},
		{
			name:     "wrapped net.Error is transient",
			err:      fmt.Errorf("request failed: %w", &net.OpError{Op: "dial", Err: fmt.Errorf("connection refused")}),
			expected: true,
		},
		{
			name:     "generic error is not transient",
			err:      fmt.Errorf("invalid_grant: token expired"),
			expected: false,
		},
		{
			name:     "sentinel error is not transient",
			err:      errUtils.ErrWebflowTokenExchange,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTransientTokenError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRefreshWebflowCredentials_TransientErrorPreservesCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Save a valid refresh cache.
	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "still-valid-token",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(10 * time.Hour),
	})

	// Mock endpoint that simulates a network error (transient).
	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(_ *http.Request) (*http.Response, error) {
			return nil, &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: fmt.Errorf("connection refused"),
			}
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)

	// Cache should still exist because the error was transient.
	cache, cacheErr := identity.loadRefreshCache()
	require.NoError(t, cacheErr)
	assert.Equal(t, "still-valid-token", cache.RefreshToken)
}

// mockHTTPClient is a test helper for mocking HTTP requests.
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}
