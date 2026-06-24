package pro

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExchangeOIDCToken covers the exported wrapper reused by the atmos/pro auth provider.
func TestExchangeOIDCToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/auth/github-oidc", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true, "data": {"token": "exchanged-atmos-token"}}`))
	}))
	defer server.Close()

	token, err := ExchangeOIDCToken(server.URL, "api", "github-oidc-token", "ws-1")
	require.NoError(t, err)
	assert.Equal(t, "exchanged-atmos-token", token)
}

// TestMintGitHubOIDCToken_NotInActions covers the exported wrapper when the GitHub Actions
// OIDC request environment is absent.
func TestMintGitHubOIDCToken_NotInActions(t *testing.T) {
	_, err := MintGitHubOIDCToken(schema.GithubOIDCSettings{}, "custom-aud")
	require.ErrorIs(t, err, errUtils.ErrNotInGitHubActions)
}

// TestGetGitHubOIDCTokenWithAudience_DefaultAudience covers the empty-audience default branch,
// asserting the request carries the default Atmos Pro audience.
func TestGetGitHubOIDCTokenWithAudience_DefaultAudience(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "audience="+DefaultProAudience)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value": "github-oidc-token"}`))
	}))
	defer server.Close()

	settings := schema.GithubOIDCSettings{
		RequestURL:   server.URL + "?token=dummy",
		RequestToken: "test-request-token",
	}

	token, err := getGitHubOIDCTokenWithAudience(settings, "", server.Client())
	require.NoError(t, err)
	assert.Equal(t, "github-oidc-token", token)
}
