package aws

// Tests for token endpoint HTTP exchange, response parsing, and OAuth error classification (webflow_token.go).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestExchangeCodeForCredentials_Success(t *testing.T) {
	// Mock token endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

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

	resp, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "auth-code-123", codeVerifier: "code-verifier-abc", redirectURI: "http://127.0.0.1:8080/oauth/callback",
	})

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

	resp, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "code", codeVerifier: "verifier", redirectURI: "http://127.0.0.1:8080/oauth/callback",
	})

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

	resp, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "expired-code", codeVerifier: "verifier", redirectURI: "http://127.0.0.1:8080/oauth/callback",
	})

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

	resp, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "code", codeVerifier: "verifier", redirectURI: "http://127.0.0.1:8080/oauth/callback",
	})

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

func TestExchangeRefreshToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)

		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, webflowGrantTypeRefresh, r.FormValue("grant_type"))
		assert.Equal(t, "my-refresh-token", r.FormValue("refresh_token"))

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

func TestCallTokenEndpoint_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "code")

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
	assert.Contains(t, err.Error(), "parse token response")
}

func TestCallTokenEndpoint_NonOK_NoErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "code")

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

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "expired-code")

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

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "code")

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

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "code")

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
	assert.Contains(t, err.Error(), "missing credentials")
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

func TestCallTokenEndpoint_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

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

	body := url.Values{}
	body.Set("client_id", webflowOAuthClientID)
	body.Set("grant_type", webflowGrantTypeAuthCode)
	body.Set("code", "auth-code")

	resp, err := callTokenEndpoint(context.Background(), mockClient, "us-east-2", body)
	require.NoError(t, err)
	assert.Equal(t, "AKID_DIRECT", resp.AccessToken.AccessKeyID)
	assert.Equal(t, "SECRET_DIRECT", resp.AccessToken.SecretAccessKey)
	assert.Equal(t, "TOKEN_DIRECT", resp.AccessToken.SessionToken)
	assert.Equal(t, 900, resp.ExpiresIn)
	assert.Equal(t, "refresh-direct", resp.RefreshToken)
}

func TestIsDefinitiveOAuthError(t *testing.T) {
	tests := []struct {
		name     string
		oauthErr string
		expected bool
	}{
		{name: "invalid_grant is definitive", oauthErr: "invalid_grant", expected: true},
		{name: "invalid_token is definitive", oauthErr: "invalid_token", expected: true},
		{name: "invalid_request is not definitive", oauthErr: "invalid_request", expected: false},
		{name: "invalid_client is not definitive", oauthErr: "invalid_client", expected: false},
		{name: "unauthorized_client is not definitive", oauthErr: "unauthorized_client", expected: false},
		{name: "unsupported_grant_type is not definitive", oauthErr: "unsupported_grant_type", expected: false},
		{name: "invalid_scope is not definitive", oauthErr: "invalid_scope", expected: false},
		{name: "server_error is not definitive", oauthErr: "server_error", expected: false},
		{name: "temporarily_unavailable is not definitive", oauthErr: "temporarily_unavailable", expected: false},
		{name: "empty string is not definitive", oauthErr: "", expected: false},
		{name: "unknown code is not definitive", oauthErr: "mystery_error", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isDefinitiveOAuthError(tt.oauthErr))
		})
	}
}

// TestExchangeCodeForCredentials_InvalidGrantNotWrappedAsRevoked verifies that
// when the authorization_code grant (not refresh_token) returns invalid_grant,
// the error is NOT wrapped with ErrWebflowRefreshTokenRevoked. The sentinel is
// scoped to refresh-token rejection so callers don't misinterpret a stale auth
// code as a revoked refresh token.
func TestExchangeCodeForCredentials_InvalidGrantNotWrappedAsRevoked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "authorization code expired",
		})
	}))
	defer server.Close()

	mockClient := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			req.URL, _ = url.Parse(server.URL + req.URL.Path)
			return http.DefaultClient.Do(req)
		},
	}

	resp, err := exchangeCodeForCredentials(context.Background(), mockClient, exchangeCodeParams{
		region: "us-east-2", code: "expired-code", codeVerifier: "verifier", redirectURI: "http://127.0.0.1:8080/oauth/callback",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, errUtils.ErrWebflowTokenExchange)
	assert.NotErrorIs(t, err, errUtils.ErrWebflowRefreshTokenRevoked,
		"authorization_code flow must not produce ErrWebflowRefreshTokenRevoked")
}
