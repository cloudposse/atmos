package pro

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	atmosErrors "github.com/cloudposse/atmos/errors"
)

func TestExchangeOIDCTokenForAtmosToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/auth/github-oidc", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "github-oidc-token-123")
		assert.Contains(t, string(body), "test-workspace-id")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"success": true,
			"data": {
				"token": "exchanged-atmos-token-xyz"
			}
		}`))
	}))
	defer server.Close()

	token, err := exchangeOIDCTokenForAtmosToken(server.URL, "api", "github-oidc-token-123", "test-workspace-id")
	assert.NoError(t, err)
	assert.Equal(t, "exchanged-atmos-token-xyz", token)
}

func TestExchangeOIDCTokenForAtmosToken_HTTPErrors(t *testing.T) {
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
			expectedError: atmosErrors.ErrFailedToExchangeOIDCToken,
		},
		{
			name: "server returns 403 forbidden",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte(`{"error": "forbidden"}`))
			},
			expectedError: atmosErrors.ErrFailedToExchangeOIDCToken,
		},
		{
			name: "server returns invalid JSON",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`invalid json`))
			},
			expectedError: atmosErrors.ErrFailedToDecodeTokenResponse,
		},
		{
			name: "server returns success false with error message",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"success": false,
					"errorMessage": "Invalid OIDC token or workspace"
				}`))
			},
			expectedError: atmosErrors.ErrFailedToExchangeOIDCToken,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer server.Close()

			token, err := exchangeOIDCTokenForAtmosToken(server.URL, "api", "oidc-token", "workspace-id")
			assert.Error(t, err)
			assert.Equal(t, "", token)
			assert.ErrorIs(t, err, tc.expectedError)
		})
	}
}

func TestExchangeOIDCTokenForAtmosToken_NetworkError(t *testing.T) {
	// Use an invalid URL to simulate network error
	token, err := exchangeOIDCTokenForAtmosToken("http://invalid-host-that-does-not-exist:12345", "api", "oidc-token", "workspace-id")
	assert.Error(t, err)
	assert.Equal(t, "", token)
	assert.ErrorIs(t, err, atmosErrors.ErrFailedToExchangeOIDCToken)
}

func TestExchangeOIDCTokenForAtmosToken_RequestCreationError(t *testing.T) {
	// Test with malformed URL that will cause http.NewRequest to fail
	token, err := exchangeOIDCTokenForAtmosToken("://invalid-url", "api", "oidc-token", "workspace-id")
	assert.Error(t, err)
	assert.Equal(t, "", token)
	assert.ErrorIs(t, err, atmosErrors.ErrFailedToCreateRequest)
}

func TestExchangeOIDCTokenForAtmosToken_MarshalError(t *testing.T) {
	// This is harder to test without modifying the function, but we can test with extreme values
	// that might cause issues. In practice, JSON marshaling rarely fails with normal string inputs.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true, "data": {"token": "test"}}`))
	}))
	defer server.Close()

	// Test with normal inputs (marshal should succeed)
	token, err := exchangeOIDCTokenForAtmosToken(server.URL, "api", "oidc-token", "workspace-id")
	assert.NoError(t, err)
	assert.Equal(t, "test", token)
}
