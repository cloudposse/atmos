package pro

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetGitHubOIDCToken_Success(t *testing.T) {
	// Save original env vars
	originalURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	originalToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	defer func() {
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", originalURL)
		os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", originalToken)
	}()

	// Set up TLS test server (HTTPS required by SSRF URL validation).
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
		// Verify audience parameter is added
		assert.Contains(t, r.URL.RawQuery, "audience=atmos-pro.com")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"value": "github-oidc-token-123"}`))
	}))
	defer server.Close()

	// Create GitHub OIDC settings directly, injecting TLS-aware client.
	githubOIDCSettings := schema.GithubOIDCSettings{
		RequestURL:   server.URL + "?token=dummy",
		RequestToken: "test-request-token",
	}

	token, err := getGitHubOIDCToken(githubOIDCSettings, server.Client())
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
	}()

	testCases := []struct {
		name               string
		githubOIDCSettings schema.GithubOIDCSettings
	}{
		{
			name: "missing REQUEST_URL",
			githubOIDCSettings: schema.GithubOIDCSettings{
				RequestURL:   "",
				RequestToken: "test-token",
			},
		},
		{
			name: "missing REQUEST_TOKEN",
			githubOIDCSettings: schema.GithubOIDCSettings{
				RequestURL:   "http://example.com",
				RequestToken: "",
			},
		},
		{
			name: "both missing",
			githubOIDCSettings: schema.GithubOIDCSettings{
				RequestURL:   "",
				RequestToken: "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := getGitHubOIDCToken(tc.githubOIDCSettings)
			assert.Error(t, err)
			assert.Equal(t, "", token)
			assert.ErrorIs(t, err, errUtils.ErrNotInGitHubActions)
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
	}()

	// Set up TLS test server that returns an error (HTTPS required by SSRF URL validation).
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	t.Run("http error response", func(t *testing.T) {
		githubOIDCSettings := schema.GithubOIDCSettings{
			RequestURL:   server.URL + "?token=dummy",
			RequestToken: "test-token",
		}

		token, err := getGitHubOIDCToken(githubOIDCSettings, server.Client())
		assert.Error(t, err)
		assert.Equal(t, "", token)
		assert.ErrorIs(t, err, errUtils.ErrFailedToGetOIDCToken)
	})
}

func TestGetGitHubOIDCToken_NetworkError(t *testing.T) {
	githubOIDCSettings := schema.GithubOIDCSettings{
		// Use https:// scheme so URL validation passes; connection will fail at the network level.
		RequestURL:   "https://invalid-host-that-does-not-exist:12345?token=dummy",
		RequestToken: "test-token",
	}

	token, err := getGitHubOIDCToken(githubOIDCSettings)
	assert.Error(t, err)
	assert.Equal(t, "", token)
	assert.ErrorIs(t, err, errUtils.ErrFailedToGetOIDCToken)
}

func TestGetGitHubOIDCToken_URLValidation(t *testing.T) {
	tests := []struct {
		name        string
		requestURL  string
		expectedErr error
	}{
		{
			name:        "http scheme rejected",
			requestURL:  "http://token.actions.githubusercontent.com/api/v1/token",
			expectedErr: errUtils.ErrFailedToGetGitHubOIDCToken,
		},
		{
			name:        "empty host rejected",
			requestURL:  "https:///api/v1/token",
			expectedErr: errUtils.ErrFailedToGetGitHubOIDCToken,
		},
		{
			name:        "invalid URL rejected",
			requestURL:  "://bad-url",
			expectedErr: errUtils.ErrFailedToGetGitHubOIDCToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := schema.GithubOIDCSettings{
				RequestURL:   tt.requestURL,
				RequestToken: "test-token",
			}
			token, err := getGitHubOIDCToken(settings)
			assert.Error(t, err)
			assert.Equal(t, "", token)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}
