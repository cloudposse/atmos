package aws

// Tests for browser webflow dispatch, interactive/non-interactive flows, wait loops, and processTokenResponse (webflow_browser.go).

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

	creds := identity.processTokenResponse(tokenResp, "us-west-2")
	require.NotNil(t, creds)
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

	creds := identity.processTokenResponse(tokenResp, "eu-west-1")
	require.NotNil(t, creds)
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
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshTokenRevoked)

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
			resp, err := http.Get(callbackURL)
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
			resp, err := http.Get(callbackURL)
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

// TestRefreshWebflowCredentials_TransientFailuresPreserveCache verifies that
// server-side and client-side transient failures do NOT delete the cached refresh
// token. Only a definitive OAuth2 rejection (HTTP 400 invalid_grant/invalid_token)
// should clear the cache — everything else (HTTP 5xx, 429, network errors,
// malformed JSON, context cancellation) must preserve it so unattended runs can
// retry without forcing a browser prompt.
func TestRefreshWebflowCredentials_TransientFailuresPreserveCache(t *testing.T) {
	jsonErrHandler := func(status int, errCode string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             errCode,
				"error_description": "simulated transient failure",
			})
		}
	}

	cases := []refreshCachePreservationTestCase{
		{
			name:    "HTTP 500 server error preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		},
		{
			name:    "HTTP 502 bad gateway preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		},
		{
			name:    "HTTP 503 service unavailable preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
		},
		{
			name:    "HTTP 504 gateway timeout preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGatewayTimeout) },
		},
		{
			name:    "HTTP 429 too many requests preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
		},
		{
			name:    "HTTP 400 server_error OAuth code preserves cache",
			handler: jsonErrHandler(http.StatusBadRequest, "server_error"),
		},
		{
			name:    "HTTP 400 temporarily_unavailable OAuth code preserves cache",
			handler: jsonErrHandler(http.StatusBadRequest, "temporarily_unavailable"),
		},
		{
			name:    "HTTP 400 invalid_request (non-definitive) preserves cache",
			handler: jsonErrHandler(http.StatusBadRequest, "invalid_request"),
		},
		{
			name: "malformed JSON 200 response preserves cache",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("{not-json"))
			},
		},
		{
			name:     "network error preserves cache",
			netError: &net.OpError{Op: "dial", Net: "tcp", Err: fmt.Errorf("connection refused")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

			identity := &userIdentity{
				name:   "test",
				realm:  "realm",
				config: &schema.Identity{Kind: "aws/user"},
			}

			// Seed cache with a valid (non-expired) refresh token.
			identity.saveRefreshCache(&webflowRefreshCache{
				RefreshToken: "should-survive",
				Region:       "us-east-2",
				ExpiresAt:    time.Now().Add(10 * time.Hour),
			})

			origClient := defaultHTTPClient
			defer func() { defaultHTTPClient = origClient }()

			if tc.netError != nil {
				defaultHTTPClient = &mockHTTPClient{
					doFunc: func(_ *http.Request) (*http.Response, error) { return nil, tc.netError },
				}
			} else {
				server := httptest.NewServer(tc.handler)
				defer server.Close()
				defaultHTTPClient = &mockHTTPClient{
					doFunc: func(req *http.Request) (*http.Response, error) {
						req.URL, _ = url.Parse(server.URL + req.URL.Path)
						return http.DefaultClient.Do(req)
					},
				}
			}

			creds, err := identity.refreshWebflowCredentials(context.Background(), "us-east-2")
			require.Error(t, err)
			assert.Nil(t, creds)
			assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)
			assert.NotErrorIs(t, err, errUtils.ErrWebflowRefreshTokenRevoked,
				"transient failure must not be wrapped with ErrWebflowRefreshTokenRevoked")

			// Cache must still exist with the original refresh token.
			cache, cacheErr := identity.loadRefreshCache()
			require.NoError(t, cacheErr, "refresh cache was deleted on a transient failure")
			require.NotNil(t, cache)
			assert.Equal(t, "should-survive", cache.RefreshToken)
		})
	}
}

// TestRefreshWebflowCredentials_ContextCancellationPreservesCache verifies that
// a cancelled context during refresh does not delete the cached refresh token.
func TestRefreshWebflowCredentials_ContextCancellationPreservesCache(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:   "test",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	identity.saveRefreshCache(&webflowRefreshCache{
		RefreshToken: "should-survive-cancel",
		Region:       "us-east-2",
		ExpiresAt:    time.Now().Add(10 * time.Hour),
	})

	origClient := defaultHTTPClient
	defaultHTTPClient = &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Honor the request context so cancellation propagates.
			return nil, req.Context().Err()
		},
	}
	defer func() { defaultHTTPClient = origClient }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before the call.

	creds, err := identity.refreshWebflowCredentials(ctx, "us-east-2")
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)
	assert.NotErrorIs(t, err, errUtils.ErrWebflowRefreshTokenRevoked)

	cache, cacheErr := identity.loadRefreshCache()
	require.NoError(t, cacheErr)
	require.NotNil(t, cache)
	assert.Equal(t, "should-survive-cancel", cache.RefreshToken)
}

// TestRefreshWebflowCredentials_InvalidGrantDeletesCache verifies that a
// definitive OAuth2 rejection (invalid_grant) on the refresh_token grant
// deletes the cached refresh token and wraps the error with
// ErrWebflowRefreshTokenRevoked.
func TestRefreshWebflowCredentials_InvalidGrantDeletesCache(t *testing.T) {
	tests := []struct {
		name     string
		oauthErr string
	}{
		{name: "invalid_grant deletes cache", oauthErr: "invalid_grant"},
		{name: "invalid_token deletes cache", oauthErr: "invalid_token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

			identity := &userIdentity{
				name:   "test",
				realm:  "realm",
				config: &schema.Identity{Kind: "aws/user"},
			}

			identity.saveRefreshCache(&webflowRefreshCache{
				RefreshToken: "revoked-token",
				Region:       "us-east-2",
				ExpiresAt:    time.Now().Add(10 * time.Hour),
			})

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error":             tt.oauthErr,
					"error_description": "token has been revoked",
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
			require.Error(t, err)
			assert.Nil(t, creds)
			assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshFailed)
			assert.ErrorIs(t, err, errUtils.ErrWebflowRefreshTokenRevoked)

			// Cache MUST be deleted.
			cache, cacheErr := identity.loadRefreshCache()
			assert.Nil(t, cache)
			assert.Error(t, cacheErr, "refresh cache should have been deleted on definitive rejection")
		})
	}
}

// TestBrowserWebflowNonInteractive_CallbackSuccess verifies that
// browserWebflowNonInteractive succeeds when the callback server receives the
// authorization code before stdin produces input (the `resultCh` branch of the
// select wins the race). This exercises all of the non-interactive setup code
// plus the callback→token-exchange→processTokenResponse path.
func TestBrowserWebflowNonInteractive_CallbackSuccess(t *testing.T) {
	blockStdin(t)
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test-noninteractive",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-2",
			},
		},
	}

	// Mock token endpoint returns valid credentials with a refresh token so
	// processTokenResponse also saves the cache (exercises saveRefreshCache).
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_NI",
				"secretAccessKey": "SECRET_NI",
				"sessionToken":    "TOKEN_NI",
			},
			"expiresIn":    900,
			"refreshToken": "refresh-ni",
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

	// Capture the auth URL via the mockable plain-text display and fire the
	// callback on a background goroutine. The callback hits the ephemeral
	// callback server and should win the select race against stdin input.
	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		go func() {
			time.Sleep(50 * time.Millisecond)
			simulateCallback(t, authURL, "ni-callback-code")
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_NI", creds.AccessKeyID)
	assert.Equal(t, "SECRET_NI", creds.SecretAccessKey)
	assert.Equal(t, "TOKEN_NI", creds.SessionToken)

	// Verify refresh token was cached.
	cache, cacheErr := identity.loadRefreshCache()
	require.NoError(t, cacheErr)
	assert.Equal(t, "refresh-ni", cache.RefreshToken)
}

// TestBrowserWebflowNonInteractive_CallbackServerError verifies that a
// callback arriving with an error parameter surfaces as ErrWebflowAuthFailed.
func TestBrowserWebflowNonInteractive_CallbackServerError(t *testing.T) {
	blockStdin(t)
	identity := &userIdentity{
		name:  "test-ni-callback-err",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		// Fire a callback with error=access_denied on a goroutine.
		go func() {
			time.Sleep(30 * time.Millisecond)
			parsed, _ := url.Parse(authURL)
			redirectURI := parsed.Query().Get("redirect_uri")
			state := parsed.Query().Get("state")
			errURL := fmt.Sprintf("%s?error=access_denied&error_description=user%%20cancelled&state=%s", redirectURI, state)
			resp, err := http.Get(errURL)
			if err == nil {
				resp.Body.Close()
			}
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	assert.Nil(t, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}

// TestBrowserWebflowNonInteractive_ContextCancelled verifies the context-done
// branch of the select: cancellation returns ErrWebflowTimeout and produces no
// credentials. Stdin is never exercised because we cancel before either the
// callback or stdin can fire.
func TestBrowserWebflowNonInteractive_ContextCancelled(t *testing.T) {
	blockStdin(t)
	identity := &userIdentity{
		name:  "test-ni-cancel",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// No-op display: nothing will fire a callback.
	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(_ string) {}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay to ensure the function has entered the select.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	assert.Nil(t, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTimeout)
}

// TestBrowserWebflowNonInteractive_TokenExchangeFails verifies that a failing
// token exchange after a successful callback surfaces the exchange error
// (exercises the non-nil tokenErr branch in the callback case).
func TestBrowserWebflowNonInteractive_TokenExchangeFails(t *testing.T) {
	blockStdin(t)
	identity := &userIdentity{
		name:  "test-ni-exchange-fail",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	// Mock token endpoint returns HTTP 500 (transient) so it wraps
	// ErrWebflowTokenExchange, not the revoked sentinel.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		go func() {
			time.Sleep(30 * time.Millisecond)
			simulateCallback(t, authURL, "code-that-fails-exchange")
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	assert.Nil(t, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
}

// TestBrowserWebflow_DispatchesToNonInteractive verifies the browserWebflow
// dispatch logic: when prompts are allowed but TTY is unavailable, it routes
// to browserWebflowNonInteractive. This covers the `if allowPrompts { ... }`
// branch at webflow.go:188-190.
func TestBrowserWebflow_DispatchesToNonInteractive(t *testing.T) {
	blockStdin(t)
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test-dispatch-ni",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_DISPATCH",
				"secretAccessKey": "SECRET_DISPATCH",
				"sessionToken":    "TOKEN_DISPATCH",
			},
			"expiresIn": 900,
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

	// Force non-TTY so browserWebflow routes to the non-interactive branch.
	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		go func() {
			time.Sleep(30 * time.Millisecond)
			simulateCallback(t, authURL, "dispatch-code")
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.browserWebflow(ctx, "us-east-2")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_DISPATCH", creds.AccessKeyID)
}

// TestResolveCredentialsViaWebflow_FallsThroughToBrowser verifies that when
// refresh cache is missing, resolveCredentialsViaWebflow falls through to
// browserWebflow (the happy path). Covers the `err != nil` branch after
// refreshWebflowCredentials fails.
func TestResolveCredentialsViaWebflow_FallsThroughToBrowser(t *testing.T) {
	blockStdin(t)
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:  "test-fallthrough",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-2",
			},
		},
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_FT",
				"secretAccessKey": "SECRET_FT",
				"sessionToken":    "TOKEN_FT",
			},
			"expiresIn": 900,
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

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		go func() {
			time.Sleep(30 * time.Millisecond)
			simulateCallback(t, authURL, "fallthrough-code")
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.resolveCredentialsViaWebflow(ctx)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_FT", creds.AccessKeyID)
}

// TestBrowserWebflowNonInteractive_StdinCodeEntry verifies the `codeCh` branch
// of the select: when stdin produces a valid authorization code before the
// callback server receives a request, the function exchanges that code for
// tokens.
func TestBrowserWebflowNonInteractive_StdinCodeEntry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	// Inject a controlled reader via the package override and write the code
	// after a short delay so the function has time to enter the select.
	w := stdinReaderWith(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte("stdin-entered-code\n"))
	}()

	identity := &userIdentity{
		name:  "test-ni-stdin",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		require.NoError(t, req.ParseForm())
		assert.Equal(t, "stdin-entered-code", req.FormValue("code"))
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_STDIN",
				"secretAccessKey": "SECRET_STDIN",
				"sessionToken":    "TOKEN_STDIN",
			},
			"expiresIn": 900,
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

	// No-op display to ensure the callback server does NOT receive a request.
	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(_ string) {}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_STDIN", creds.AccessKeyID)
}

// TestBrowserWebflowNonInteractive_EmptyStdinCode verifies the errCh branch:
// when stdin produces an empty line, the function surfaces an authorization-
// code-required error wrapped in ErrWebflowAuthFailed.
func TestBrowserWebflowNonInteractive_EmptyStdinCode(t *testing.T) {
	w := stdinReaderWith(t)
	go func() {
		time.Sleep(30 * time.Millisecond)
		_, _ = w.Write([]byte("   \n")) // Whitespace only → TrimSpace → empty.
	}()

	identity := &userIdentity{
		name:  "test-ni-empty",
		realm: "realm",
		config: &schema.Identity{
			Kind: "aws/user",
		},
	}

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(_ string) {}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	assert.Nil(t, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
	assert.Contains(t, err.Error(), "authorization code is required")
}

// TestBrowserWebflow_ForceTTYDispatchesToInteractive verifies that when
// viper's force-tty flag is set, browserWebflow routes to the interactive
// path even if the underlying TTY check would say otherwise. This honors
// --force-tty / ATMOS_FORCE_TTY=true for screenshot generation and similar
// workflows where auto-detection fails.
func TestBrowserWebflow_ForceTTYDispatchesToInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	// Force TTY via viper; webflowIsTTYFunc still returns false so only the
	// force-tty branch can lead to the interactive path.
	viper.Set("force-tty", true)
	defer viper.Set("force-tty", false)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	// Trace which internal function was called by mocking the spinner runner
	// to observe whether browserWebflow reached the interactive branch
	// (interactive path calls runSpinnerProgramFunc inside
	// waitForCallbackWithSpinner).
	interactiveReached := false
	origRun := runSpinnerProgramFunc
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		interactiveReached = true
		model.done = true
		model.result = &webflowSpinnerTokenResult{
			resp: &webflowTokenResponse{
				AccessToken: webflowAccessToken{AccessKeyID: "AKID_FORCE_TTY", SecretAccessKey: "S"},
				ExpiresIn:   900,
			},
		}
		return model, nil
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	// Unset CI env vars so waitForCallbackWithSpinner does NOT divert to the
	// simple path for CI reasons. This test isolates the force-tty effect.
	unsetCIEnvVars(t)

	// Mock display dialog to avoid writing to stderr during tests.
	origDisplay := displayWebflowDialogFunc
	displayWebflowDialogFunc = func(_ string) {}
	defer func() { displayWebflowDialogFunc = origDisplay }()

	// Mock openURLFunc to avoid attempting to launch a real browser.
	origOpen := openURLFunc
	openURLFunc = func(_ string) error { return nil }
	defer func() { openURLFunc = origOpen }()

	identity := &userIdentity{
		name:   "test-force-tty",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.browserWebflow(ctx, "us-east-2")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_FORCE_TTY", creds.AccessKeyID)
	assert.True(t, interactiveReached, "force-tty should have routed browserWebflow to the interactive path + spinner")
}

// TestWaitForCallbackWithSpinner_ForceTTYOutsideCI verifies that the spinner
// branch is taken when force-tty is set AND we are not in CI, even if the
// underlying TTY check returns false.
func TestWaitForCallbackWithSpinner_ForceTTYOutsideCI(t *testing.T) {
	unsetCIEnvVars(t)

	viper.Set("force-tty", true)
	defer viper.Set("force-tty", false)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	spinnerRan := false
	origRun := runSpinnerProgramFunc
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		spinnerRan = true
		model.done = true
		model.result = &webflowSpinnerTokenResult{
			resp: &webflowTokenResponse{
				AccessToken: webflowAccessToken{AccessKeyID: "AKID_FT_SP", SecretAccessKey: "S"},
			},
		}
		return model, nil
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	identity := &userIdentity{
		name:   "test-ft-spinner",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	resultCh := make(chan webflowResult)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := identity.waitForCallbackWithSpinner(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_FT_SP", resp.AccessToken.AccessKeyID)
	assert.True(t, spinnerRan, "force-tty should have enabled the spinner branch outside CI")
}

// TestWaitForCallbackWithSpinner_ForceTTYInsideCISuppressesSpinner verifies
// that CI suppression wins over force-tty: when CI is detected, the spinner
// does NOT run even if force-tty is set. This protects CI logs from being
// spammed with spinner escape sequences when a real TTY is attached.
func TestWaitForCallbackWithSpinner_ForceTTYInsideCISuppressesSpinner(t *testing.T) {
	unsetCIEnvVars(t)

	viper.Set("force-tty", true)
	defer viper.Set("force-tty", false)

	// Simulate CI environment.
	t.Setenv("CI", "true")

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return false }
	defer func() { webflowIsTTYFunc = origTTY }()

	spinnerRan := false
	origRun := runSpinnerProgramFunc
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		spinnerRan = true
		return model, nil
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	// Set up a token endpoint so the simple path can succeed.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId": "AKID_CI_SIMPLE", "secretAccessKey": "S", "sessionToken": "T",
			},
			"expiresIn": 900,
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

	identity := &userIdentity{
		name:   "test-ft-ci",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{code: "ci-simple-code", state: "state"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := identity.waitForCallbackWithSpinner(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_CI_SIMPLE", resp.AccessToken.AccessKeyID)
	assert.False(t, spinnerRan, "CI should suppress the spinner even when force-tty is set")
}

// unsetCIEnvVars removes every CI-detection environment variable telemetry.IsCI
// inspects so that tests wanting to reach the TTY branch of
// waitForCallbackWithSpinner do not get short-circuited to the simple path.
// Note that t.Setenv(...,"") does NOT work here because telemetry uses
// os.LookupEnv, which returns ok=true for empty-string values.
func unsetCIEnvVars(t *testing.T) {
	t.Helper()
	for _, envVar := range []string{
		"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "TRAVIS", "JENKINS_URL",
		"BUILDKITE", "APPVEYOR", "DRONE", "TEAMCITY_VERSION", "BITBUCKET_BUILD_NUMBER",
		"BITRISE_BUILD_URL", "CODEBUILD_BUILD_ID", "TF_BUILD", "SEMAPHORE",
	} {
		orig, had := os.LookupEnv(envVar)
		_ = os.Unsetenv(envVar)
		if had {
			t.Cleanup(func() { _ = os.Setenv(envVar, orig) })
		}
	}
}

// TestWaitForCallbackWithSpinner_SpinnerSuccess exercises the TTY success
// branch of waitForCallbackWithSpinner by stubbing runSpinnerProgramFunc to
// simulate bubbletea returning a completed model with a populated result.
// The function must forward that result's response/error back to the caller.
func TestWaitForCallbackWithSpinner_SpinnerSuccess(t *testing.T) {
	unsetCIEnvVars(t)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return true }
	defer func() { webflowIsTTYFunc = origTTY }()

	origRun := runSpinnerProgramFunc
	wantResp := &webflowTokenResponse{
		AccessToken: webflowAccessToken{
			AccessKeyID:     "AKID_SPINNER_OK",
			SecretAccessKey: "SECRET_SPINNER_OK",
			SessionToken:    "TOKEN_SPINNER_OK",
		},
		ExpiresIn: 900,
	}
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		// Simulate the tea update loop delivering the token-result message.
		model.done = true
		model.result = &webflowSpinnerTokenResult{resp: wantResp}
		return model, nil
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	identity := &userIdentity{
		name:   "test-spinner-success",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// resultCh never needs to fire because the stubbed spinner produces the
	// final state directly. The goroutine inside
	// startSpinnerExchangeGoroutine will block on <-resultCh / <-ctx.Done(),
	// which is fine: the outer function returns based on finalModel alone.
	resultCh := make(chan webflowResult)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := identity.waitForCallbackWithSpinner(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_SPINNER_OK", resp.AccessToken.AccessKeyID)
	assert.Equal(t, "SECRET_SPINNER_OK", resp.AccessToken.SecretAccessKey)
}

// TestWaitForCallbackWithSpinner_SpinnerNilResult exercises the
// finalModel.result == nil branch: the spinner exits cleanly (no run error)
// but left the model's result unset. The function must surface
// ErrWebflowAuthFailed.
func TestWaitForCallbackWithSpinner_SpinnerNilResult(t *testing.T) {
	unsetCIEnvVars(t)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return true }
	defer func() { webflowIsTTYFunc = origTTY }()

	origRun := runSpinnerProgramFunc
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		model.done = true
		// Deliberately leave model.result == nil.
		return model, nil
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	identity := &userIdentity{
		name:   "test-spinner-nil-result",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	resultCh := make(chan webflowResult)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := identity.waitForCallbackWithSpinner(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}

// TestWaitForCallbackWithSpinner_SpinnerFallback exercises the fallback path:
// runSpinnerProgramFunc returns an error, triggering handleSpinnerFallback
// which drains the exchange goroutine. To make the drain deterministic the
// test pre-populates resultCh *and* uses a real token server, and crucially
// uses a synchronization channel to ensure the exchange goroutine completes
// (writes its result to tokenCh) BEFORE the fake spinner returns its error.
// This eliminates the previously-observed race between resultCh being read
// and the context being cancelled.
func TestWaitForCallbackWithSpinner_SpinnerFallback(t *testing.T) {
	unsetCIEnvVars(t)

	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { return true }
	defer func() { webflowIsTTYFunc = origTTY }()

	identity := &userIdentity{
		name:   "test-spinner-fallback",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Real mock token endpoint — exchange must succeed and return creds.
	exchangeStarted := make(chan struct{}, 1)
	exchangeDone := make(chan struct{}, 1)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case exchangeStarted <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_FB",
				"secretAccessKey": "SECRET_FB",
				"sessionToken":    "TOKEN_FB",
			},
			"expiresIn": 900,
		})
		select {
		case exchangeDone <- struct{}{}:
		default:
		}
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

	// Fake spinner: wait for the exchange goroutine to finish its work,
	// THEN return an error so handleSpinnerFallback drains a known-good
	// tokenCh (no race between resultCh and context cancellation).
	//
	// IMPORTANT: the wait on exchangeDone is bounded to prevent the test
	// from hanging for the full parent context deadline if the exchange
	// never runs (e.g. a Windows-specific httptest/scheduling quirk). If
	// the timeout fires, the stub still returns, handleSpinnerFallback
	// drains tokenCh (which may be empty), and the test fails fast with a
	// clear error rather than hanging the whole CI job.
	origRun := runSpinnerProgramFunc
	runSpinnerProgramFunc = func(model webflowSpinnerModel) (tea.Model, error) {
		select {
		case <-exchangeDone:
			// Give the goroutine a tick to actually write to tokenCh after
			// the HTTP response is sent (tokenCh send happens after Do()
			// returns).
			time.Sleep(20 * time.Millisecond)
		case <-time.After(2 * time.Second):
			// Exchange goroutine never signaled completion. Proceed anyway
			// so the test can fail cleanly with a diagnostic rather than
			// hang.
		}
		return model, fmt.Errorf("simulated tea run failure")
	}
	defer func() { runSpinnerProgramFunc = origRun }()

	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{code: "fallback-code", state: "state"}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := identity.waitForCallbackWithSpinner(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:8080/oauth/callback")
	// Ensure the token server was actually hit.
	select {
	case <-exchangeStarted:
	default:
		t.Fatal("exchange goroutine did not hit the token server")
	}
	require.NoError(t, err, "fallback drain should return the goroutine's exchanged credentials")
	require.NotNil(t, resp)
	assert.Equal(t, "AKID_FB", resp.AccessToken.AccessKeyID)
}

// TestStartSpinnerExchangeGoroutine_CallbackError verifies that a callback
// error is wrapped with ErrWebflowAuthFailed and delivered on the token
// channel (no token exchange attempted).
func TestStartSpinnerExchangeGoroutine_CallbackError(t *testing.T) {
	resultCh := make(chan webflowResult, 1)
	resultCh <- webflowResult{err: errUtils.ErrWebflowStateMismatch}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	tokenCh := startSpinnerExchangeGoroutine(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:0/oauth/callback")
	res := <-tokenCh
	require.Error(t, res.err)
	assert.Nil(t, res.resp)
	assert.ErrorIs(t, res.err, errUtils.ErrWebflowAuthFailed)
	assert.ErrorIs(t, res.err, errUtils.ErrWebflowStateMismatch)
}

// TestStartSpinnerExchangeGoroutine_ContextCancelled verifies that a cancelled
// context produces a timeout error on the token channel.
func TestStartSpinnerExchangeGoroutine_ContextCancelled(t *testing.T) {
	resultCh := make(chan webflowResult) // unbuffered, never fires

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tokenCh := startSpinnerExchangeGoroutine(ctx, resultCh, "us-east-2", "verifier", "http://127.0.0.1:0/oauth/callback")
	res := <-tokenCh
	require.Error(t, res.err)
	assert.Nil(t, res.resp)
	assert.ErrorIs(t, res.err, errUtils.ErrWebflowTimeout)
}

// TestBrowserWebflowInteractive_OpenURLFailure verifies that a failure to
// open the browser does not abort authentication — the function logs and
// continues, and the callback path still succeeds.
func TestBrowserWebflowInteractive_OpenURLFailure(t *testing.T) {
	blockStdin(t)
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", tmpDir)

	identity := &userIdentity{
		name:   "test-open-url-fail",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Unset CI env vars so the IsCI() guard doesn't skip openURLFunc.
	for _, envVar := range []string{
		"CI", "GITHUB_ACTIONS", "GITLAB_CI", "CIRCLECI", "TRAVIS", "JENKINS_URL",
		"BUILDKITE", "APPVEYOR", "DRONE", "TEAMCITY_VERSION", "BITBUCKET_BUILD_NUMBER",
		"BITRISE_BUILD_URL", "CODEBUILD_BUILD_ID", "TF_BUILD", "SEMAPHORE",
	} {
		orig, had := os.LookupEnv(envVar)
		_ = os.Unsetenv(envVar)
		if had {
			t.Cleanup(func() { _ = os.Setenv(envVar, orig) })
		}
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId": "AKID_OPEN_FAIL", "secretAccessKey": "SECRET_OPEN_FAIL", "sessionToken": "TOKEN_OPEN_FAIL",
			},
			"expiresIn": 900,
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

	// Mock TTY call count so browserWebflow dispatches to interactive but
	// waitForCallbackWithSpinner falls through to simple (avoids bubbletea).
	ttyCall := 0
	origTTY := webflowIsTTYFunc
	webflowIsTTYFunc = func() bool { ttyCall++; return ttyCall == 1 }
	defer func() { webflowIsTTYFunc = origTTY }()

	// Mock openURLFunc to return an error (browser cannot open).
	origOpen := openURLFunc
	openCalled := false
	openURLFunc = func(_ string) error {
		openCalled = true
		return fmt.Errorf("no display")
	}
	defer func() { openURLFunc = origOpen }()

	// Mock display dialog to fire the callback so the test completes.
	origDisplay := displayWebflowDialogFunc
	displayWebflowDialogFunc = func(authURL string) {
		go func() {
			time.Sleep(30 * time.Millisecond)
			simulateCallback(t, authURL, "open-fail-code")
		}()
	}
	defer func() { displayWebflowDialogFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = types.WithAllowPrompts(ctx, true)

	creds, err := identity.browserWebflowInteractive(ctx, "us-east-2")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_OPEN_FAIL", creds.AccessKeyID)
	assert.True(t, openCalled, "openURLFunc should have been invoked")
}

// TestBrowserWebflowNonInteractive_ClosedStdinAllowsCallback is a regression
// test for the bug where a closed/piped stdin would abort a valid OAuth2
// callback flow. Previously readStdinAuthCode sent ErrWebflowStdinClosed on
// errCh the moment Scanner.Scan() returned false, and the enclosing select
// picked that error before the network callback could arrive.
//
// Under the fix, either:
//  1. browserWebflowNonInteractive skips the stdin reader entirely when
//     webflowStdinIsReadableFunc reports false (the default for non-TTY
//     stdin), OR
//  2. if the reader IS started with an empty/EOF source, the goroutine exits
//     silently without sending on errCh.
//
// This test verifies path (1) — real-world CI case where stdin is not a TTY.
func TestBrowserWebflowNonInteractive_ClosedStdinAllowsCallback(t *testing.T) {
	// Do NOT call blockStdin / stdinReaderWith / overrideStdinReadable.
	// Leave webflowStdinIsReadableFunc at its default: since in `go test`
	// os.Stdin is not a TTY, defaultWebflowStdinIsReadable returns false.
	// browserWebflowNonInteractive should therefore skip the stdin goroutine
	// and wait purely on the callback.

	identity := &userIdentity{
		name:   "test-closed-stdin-callback",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	// Mock token endpoint returning valid credentials.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"accessToken": map[string]string{
				"accessKeyId":     "AKID_CLOSED_STDIN",
				"secretAccessKey": "SECRET_CLOSED_STDIN",
				"sessionToken":    "TOKEN_CLOSED_STDIN",
			},
			"expiresIn": 900,
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

	// Fire the OAuth callback asynchronously once the display function is
	// invoked (that's the signal the callback server is already listening).
	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(authURL string) {
		go func() {
			time.Sleep(30 * time.Millisecond)
			simulateCallback(t, authURL, "closed-stdin-code")
		}()
	}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	require.NoError(t, err, "callback path must succeed even with unreadable stdin")
	require.NotNil(t, creds)
	assert.Equal(t, "AKID_CLOSED_STDIN", creds.AccessKeyID)
}

// TestReadStdinAuthCode_CleanEOFExitsSilently verifies the internal contract:
// readStdinAuthCode must NOT send on errCh when Scanner.Scan() returns false
// with a nil scanner.Err() (clean EOF). The goroutine should simply exit so
// the enclosing select can continue waiting for the real OAuth callback.
func TestReadStdinAuthCode_CleanEOFExitsSilently(t *testing.T) {
	// Use a pipe whose writer we close immediately: the reader sees clean
	// EOF (not a read error).
	overrideStdinReadable(t)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := webflowStdinReader
	webflowStdinReader = r
	t.Cleanup(func() {
		_ = r.Close()
		webflowStdinReader = orig
	})
	_ = w.Close() // Clean EOF for the reader.

	codeCh, errCh := readStdinAuthCode()

	// Neither channel should fire within a short window: the goroutine must
	// have exited silently.
	select {
	case code := <-codeCh:
		t.Fatalf("unexpected code on codeCh: %q", code)
	case e := <-errCh:
		t.Fatalf("unexpected error on errCh: %v — clean EOF must not be reported", e)
	case <-time.After(200 * time.Millisecond):
		// Expected: goroutine exited silently.
	}
}

// TestBrowserWebflowNonInteractive_ReadAuthCodeFailure verifies that a
// scanner error (reader closed unexpectedly) surfaces as
// ErrWebflowReadAuthCodeFailed.
func TestBrowserWebflowNonInteractive_ReadAuthCodeFailure(t *testing.T) {
	// Use a pipe whose read end we close immediately to force scanner.Err().
	overrideStdinReadable(t)
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_ = w.Close()
	_ = r.Close() // Closed reader — Scan() returns false, Err() may be non-nil.
	orig := webflowStdinReader
	webflowStdinReader = r
	t.Cleanup(func() { webflowStdinReader = orig })

	identity := &userIdentity{
		name:   "test-ni-scanner-err",
		realm:  "realm",
		config: &schema.Identity{Kind: "aws/user"},
	}

	origDisplay := displayWebflowPlainTextFunc
	displayWebflowPlainTextFunc = func(_ string) {}
	defer func() { displayWebflowPlainTextFunc = origDisplay }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	creds, err := identity.browserWebflowNonInteractive(ctx, "us-east-2")
	assert.Nil(t, creds)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
}
