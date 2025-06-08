package pro

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetGitHubOIDCToken_Success(t *testing.T) {
	// Save original env vars
	originalURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	originalToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	defer func() {
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", originalURL)
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", originalToken)
		viper.Reset()
	}()

	// Set up test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
		// Verify audience parameter is added
		assert.Contains(t, r.URL.RawQuery, "audience=atmos-pro.com")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value": "github-oidc-token-123"}`))
	}))
	defer server.Close()

	// Set environment variables with proper query parameter format
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL+"?token=dummy")
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-request-token")
	viper.Reset()
	viper.AutomaticEnv()

	token, err := getGitHubOIDCToken()
	assert.NoError(t, err)
	assert.Equal(t, "github-oidc-token-123", token)
}

func TestGetGitHubOIDCToken_MissingEnvironmentVariables(t *testing.T) {
	// Save original env vars
	originalURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	originalToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	defer func() {
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", originalURL)
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", originalToken)
		viper.Reset()
	}()

	testCases := []struct {
		name     string
		setupEnv func()
	}{
		{
			name: "missing REQUEST_URL",
			setupEnv: func() {
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
				os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			},
		},
		{
			name: "missing REQUEST_TOKEN",
			setupEnv: func() {
				os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://example.com")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
			},
		},
		{
			name: "both missing",
			setupEnv: func() {
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")
				os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupEnv()
			viper.Reset()
			viper.AutomaticEnv()

			token, err := getGitHubOIDCToken()
			assert.Error(t, err)
			assert.Equal(t, "", token)
			assert.ErrorIs(t, err, ErrNotInGitHubActions)
		})
	}
}

func TestGetGitHubOIDCToken_HTTPErrors(t *testing.T) {
	// Save original env vars
	originalURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	originalToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	defer func() {
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", originalURL)
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", originalToken)
		viper.Reset()
	}()

	testCases := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedError  error
	}{
		{
			name: "server returns 401 unauthorized",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error": "unauthorized"}`))
			},
			expectedError: ErrFailedToGetOIDCToken,
		},
		{
			name: "server returns 500 internal server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "internal server error"}`))
			},
			expectedError: ErrFailedToGetOIDCToken,
		},
		{
			name: "server returns invalid JSON",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`invalid json`))
			},
			expectedError: ErrFailedToDecodeOIDCResponse,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer server.Close()

			os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL+"?token=dummy")
			os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
			viper.Reset()
			viper.AutomaticEnv()

			token, err := getGitHubOIDCToken()
			assert.Error(t, err)
			assert.Equal(t, "", token)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

func TestGetGitHubOIDCToken_NetworkError(t *testing.T) {
	// Save original env vars
	originalURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	originalToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	defer func() {
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", originalURL)
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", originalToken)
		viper.Reset()
	}()

	// Use an invalid URL to simulate network error
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://invalid-host-that-does-not-exist:12345?token=dummy")
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "test-token")
	viper.Reset()
	viper.AutomaticEnv()

	token, err := getGitHubOIDCToken()
	assert.Error(t, err)
	assert.Equal(t, "", token)
	assert.ErrorIs(t, err, ErrFailedToGetOIDCToken)
}
