package aws

// Tests for PKCE, state generation, authorization URL, and callback server (webflow_oauth.go).

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	s1, err := generateStateString()
	require.NoError(t, err)
	assert.NotEmpty(t, s1)

	s2, err := generateStateString()
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
	resp, err := http.Get(callbackURL)
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
	resp, err := http.Get(callbackURL)
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
	resp, err := http.Get(callbackURL)
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
	resp, err := http.Get(callbackURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	result := <-resultCh
	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "access_denied")
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
		resp, err := http.Get(fmt.Sprintf("http://%s%s", addr, webflowCallbackPath))
		if resp != nil {
			_ = resp.Body.Close()
		}
		return err != nil
	}, 2*time.Second, 50*time.Millisecond, "server should shut down after context cancellation")
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
