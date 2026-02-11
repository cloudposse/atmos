package gcp_wif

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/api/iamcredentials/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// mockIAMService implements gcp.IAMCredentialsService for testing.
type mockIAMService struct {
	resp *iamcredentials.GenerateAccessTokenResponse
	err  error
}

func (m *mockIAMService) GenerateAccessToken(_ context.Context, _ string, _ *iamcredentials.GenerateAccessTokenRequest) (*iamcredentials.GenerateAccessTokenResponse, error) {
	return m.resp, m.err
}

func TestNew(t *testing.T) {
	spec := &types.GCPWorkloadIdentityFederationProviderSpec{
		ProjectID:                  "test-project",
		ProjectNumber:              "123456789",
		WorkloadIdentityPoolID:     "my-pool",
		WorkloadIdentityProviderID: "my-provider",
	}
	p, err := New(spec)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, ProviderKind, p.Kind())
}

func TestNew_NilSpec(t *testing.T) {
	p, err := New(nil)
	require.Error(t, err)
	assert.Nil(t, p)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestProvider_Kind(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	assert.Equal(t, "gcp/workload-identity-federation", p.Kind())
}

func TestProvider_Name(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	assert.Equal(t, ProviderKind, p.Name())

	p.SetName("custom-wif")
	assert.Equal(t, "custom-wif", p.Name())
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *types.GCPWorkloadIdentityFederationProviderSpec
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: true,
			errMsg:  "spec is nil",
		},
		{
			name: "missing project_number",
			spec: &types.GCPWorkloadIdentityFederationProviderSpec{
				WorkloadIdentityPoolID:     "pool",
				WorkloadIdentityProviderID: "provider",
			},
			wantErr: true,
			errMsg:  "project_number",
		},
		{
			name: "missing pool_id",
			spec: &types.GCPWorkloadIdentityFederationProviderSpec{
				ProjectNumber:              "123",
				WorkloadIdentityProviderID: "provider",
			},
			wantErr: true,
			errMsg:  "workload_identity_pool_id",
		},
		{
			name: "missing provider_id",
			spec: &types.GCPWorkloadIdentityFederationProviderSpec{
				ProjectNumber:          "123",
				WorkloadIdentityPoolID: "pool",
			},
			wantErr: true,
			errMsg:  "workload_identity_provider_id",
		},
		{
			name: "valid spec with _id fields",
			spec: &types.GCPWorkloadIdentityFederationProviderSpec{
				ProjectNumber:              "123",
				WorkloadIdentityPoolID:     "pool",
				WorkloadIdentityProviderID: "provider",
			},
			wantErr: false,
		},
		{
			name: "valid spec with legacy pool/provider fields",
			spec: &types.GCPWorkloadIdentityFederationProviderSpec{
				ProjectNumber:            "123",
				WorkloadIdentityPool:     "pool",
				WorkloadIdentityProvider: "provider",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{spec: tt.spec}
			err := p.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetTokenFromEnv(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:                TokenSourceTypeEnvironment,
				EnvironmentVariable: "TEST_OIDC_TOKEN",
			},
		},
	}

	// Test missing env var - ensure it's unset for this test.
	t.Setenv("TEST_OIDC_TOKEN", "")
	os.Unsetenv("TEST_OIDC_TOKEN")
	_, err := p.getTokenFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	// Test with env var set.
	t.Setenv("TEST_OIDC_TOKEN", "my-oidc-token")

	token, err := p.getTokenFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "my-oidc-token", token)
}

func TestGetTokenFromEnv_RequiresExplicitEnvVar(t *testing.T) {
	// Test that environment_variable must be explicitly specified.
	// This prevents users from accidentally using ACTIONS_ID_TOKEN_REQUEST_TOKEN.
	// It is the request token, not the OIDC token itself.
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeEnvironment,
				// No EnvironmentVariable specified.
			},
		},
	}

	_, err := p.getTokenFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "environment_variable must be specified")
}

func TestGetTokenFromFile(t *testing.T) {
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-oidc-token\n"), 0o600))

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:     TokenSourceTypeFile,
				FilePath: tokenFile,
			},
		},
	}

	token, err := p.getTokenFromFile()
	require.NoError(t, err)
	assert.Equal(t, "file-oidc-token", token)
}

func TestGetTokenFromFile_NotFound(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:     TokenSourceTypeFile,
				FilePath: filepath.Join(t.TempDir(), "nonexistent", "token"),
			},
		},
	}

	_, err := p.getTokenFromFile()
	require.Error(t, err)
}

// TestGetTokenFromURL tests URL token source (e.g. GitHub Actions OIDC).
// Requires ability to bind a local port; may be skipped in restricted environments.
func TestGetTokenFromURL(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
		assert.Equal(t, "test-audience", r.URL.Query().Get("audience"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "url-oidc-token"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				RequestToken: "test-request-token",
				Audience:     "test-audience",
				AllowedHosts: []string{serverURL.Hostname()},
			},
		},
	}
	p.WithHTTPClient(server.Client())

	token, err := p.getTokenFromURL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "url-oidc-token", token)
}

func TestGetTokenFromURL_RejectsHTTP(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeURL,
				URL:  "http://example.invalid",
			},
		},
	}

	_, err := p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}

func TestEnvironment(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectID: "wif-project",
		},
	}

	env, err := p.Environment()
	require.NoError(t, err)
	assert.Equal(t, "wif-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestPrepareEnvironment(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectID: "prep-project",
		},
	}

	env, err := p.PrepareEnvironment(context.Background(), map[string]string{"PATH": "/usr/bin"})
	require.NoError(t, err)
	assert.Equal(t, "/usr/bin", env["PATH"])
	assert.Equal(t, "prep-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestDefaultScope(t *testing.T) {
	assert.Equal(t, "https://www.googleapis.com/auth/cloud-platform", DefaultScope)
}

func TestGetScopes(t *testing.T) {
	// Default scopes.
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	assert.Equal(t, []string{DefaultScope}, p.getScopes())

	// Custom scopes.
	p.spec.Scopes = []string{"scope1", "scope2"}
	assert.Equal(t, []string{"scope1", "scope2"}, p.getScopes())
}

func TestPoolIDAndProviderID_PreferIDFields(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123",
			WorkloadIdentityPoolID:     "pool-id",
			WorkloadIdentityProviderID: "provider-id",
			WorkloadIdentityPool:       "legacy-pool",
			WorkloadIdentityProvider:   "legacy-provider",
		},
	}
	assert.Equal(t, "pool-id", p.poolID())
	assert.Equal(t, "provider-id", p.providerID())
}

func TestPoolIDAndProviderID_FallbackToLegacy(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:            "123",
			WorkloadIdentityPool:     "legacy-pool",
			WorkloadIdentityProvider: "legacy-provider",
		},
	}
	assert.Equal(t, "legacy-pool", p.poolID())
	assert.Equal(t, "legacy-provider", p.providerID())
}

func TestPreAuthenticate(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	err := p.PreAuthenticate(nil)
	assert.NoError(t, err)
}

func TestPaths(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Nil(t, paths)
}

func TestLogout(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	err := p.Logout(context.Background())
	assert.NoError(t, err)
}

func TestGetFilesDisplayPath(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	assert.Equal(t, "", p.GetFilesDisplayPath())
}

func TestGetHTTPClient_Default(t *testing.T) {
	p := &Provider{spec: &types.GCPWorkloadIdentityFederationProviderSpec{}}
	client := p.getHTTPClient()
	require.NotNil(t, client)
	assert.Equal(t, TokenRequestTimeout*time.Second, client.Timeout)
}

func TestGetHTTPClient_Custom(t *testing.T) {
	custom := &http.Client{Timeout: 30 * time.Second}
	p := &Provider{
		spec:       &types.GCPWorkloadIdentityFederationProviderSpec{},
		httpClient: custom,
	}
	assert.Same(t, custom, p.getHTTPClient())
}

func TestWithHTTPClient(t *testing.T) {
	p, err := New(&types.GCPWorkloadIdentityFederationProviderSpec{
		ProjectNumber:              "123",
		WorkloadIdentityPoolID:     "pool",
		WorkloadIdentityProviderID: "prov",
	})
	require.NoError(t, err)

	custom := &http.Client{Timeout: 5 * time.Second}
	result := p.WithHTTPClient(custom)
	assert.Same(t, p, result)
	assert.Same(t, custom, p.httpClient)
}

func TestGetOIDCToken_NilTokenSource(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{},
	}
	_, err := p.getOIDCToken(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
	assert.Contains(t, err.Error(), "token_source not configured")
}

func TestGetOIDCToken_UnknownType(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: "unknown",
			},
		},
	}
	_, err := p.getOIDCToken(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
	assert.Contains(t, err.Error(), "unknown token source type")
}

func TestGetOIDCToken_DefaultTypeIsEnvironment(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:                "", // Empty defaults to environment.
				EnvironmentVariable: "TEST_WIF_OIDC",
			},
		},
	}
	t.Setenv("TEST_WIF_OIDC", "env-token")
	token, err := p.getOIDCToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-token", token)
}

func TestGetOIDCToken_File(t *testing.T) {
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "oidc-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-token"), 0o600))

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:     TokenSourceTypeFile,
				FilePath: tokenFile,
			},
		},
	}
	token, err := p.getOIDCToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "file-token", token)
}

func TestGetTokenFromFile_MissingFilePath(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeFile,
			},
		},
	}
	_, err := p.getTokenFromFile()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file_path not configured")
}

func TestGetTokenFromFile_EmptyFile(t *testing.T) {
	tmp := t.TempDir()
	tokenFile := filepath.Join(tmp, "empty-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("   \n"), 0o600))

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:     TokenSourceTypeFile,
				FilePath: tokenFile,
			},
		},
	}
	_, err := p.getTokenFromFile()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token is empty")
}

func TestGetTokenFromURL_MissingURL(t *testing.T) {
	// Ensure GitHub Actions env var is not set.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_URL")

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeURL,
			},
		},
	}
	_, err := p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token URL not configured")
}

func TestGetTokenFromURL_HostNotAllowed(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          "https://evil.example.com/token",
				AllowedHosts: []string{"good.example.com"},
			},
		},
	}
	_, err := p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestGetTokenFromURL_NonOKStatus(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": "access_denied"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				AllowedHosts: []string{serverURL.Hostname()},
			},
		},
	}
	p.WithHTTPClient(server.Client())

	_, err = p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token request failed")
}

func TestGetTokenFromURL_EmptyResponseValue(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": ""}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				AllowedHosts: []string{serverURL.Hostname()},
			},
		},
	}
	p.WithHTTPClient(server.Client())

	_, err = p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token is empty")
}

func TestHostAllowed_EmptyHost(t *testing.T) {
	u, _ := url.Parse("https:///path")
	assert.False(t, hostAllowed(u, []string{"example.com"}))
}

func TestHostAllowed_EmptyAllowedEntry(t *testing.T) {
	u, _ := url.Parse("https://example.com/path")
	// Empty entries should be skipped, but the valid one matches.
	assert.True(t, hostAllowed(u, []string{"", "  ", "example.com"}))
}

func TestHostAllowed_CaseInsensitive(t *testing.T) {
	u, _ := url.Parse("https://Example.COM/path")
	assert.True(t, hostAllowed(u, []string{"example.com"}))
}

func TestEnvironment_NoProject(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{},
	}
	env, err := p.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestPrepareEnvironment_NilInput(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{ProjectID: "proj"},
	}
	env, err := p.PrepareEnvironment(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "proj", env["GOOGLE_CLOUD_PROJECT"])
}

func TestPrepareEnvironment_NoProject(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{},
	}
	env, err := p.PrepareEnvironment(context.Background(), map[string]string{"FOO": "bar"})
	require.NoError(t, err)
	assert.Equal(t, "bar", env["FOO"])
	_, hasProject := env["GOOGLE_CLOUD_PROJECT"]
	assert.False(t, hasProject)
}

func TestGetTokenFromURL_NoBearer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no Authorization header when no token is configured.
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "no-bearer-token"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Ensure ACTIONS_ID_TOKEN_REQUEST_TOKEN is not set.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	os.Unsetenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				AllowedHosts: []string{serverURL.Hostname()},
				// No RequestToken set.
			},
		},
	}
	p.WithHTTPClient(server.Client())

	token, err := p.getTokenFromURL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "no-bearer-token", token)
}

func TestGetTokenFromURL_InvalidJSON(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				AllowedHosts: []string{serverURL.Hostname()},
			},
		},
	}
	p.WithHTTPClient(server.Client())

	_, err = p.getTokenFromURL(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

func TestGetTokenFromURL_FromEnvVar(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "env-url-token"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Set ACTIONS_ID_TOKEN_REQUEST_URL to point to test server.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", server.URL)

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeURL,
				// No URL set — should fall back to env var.
				AllowedHosts: []string{serverURL.Hostname()},
			},
		},
	}
	p.WithHTTPClient(server.Client())

	token, err := p.getTokenFromURL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "env-url-token", token)
}

func TestExchangeToken_SuccessWithMock(t *testing.T) {
	// Start a TLS server that responds like the STS endpoint.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "federated-access-token", "expires_in": 3600, "token_type": "Bearer"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123456",
			WorkloadIdentityPoolID:     "my-pool",
			WorkloadIdentityProviderID: "my-provider",
		},
		stsURL: server.URL,
	}
	p.WithHTTPClient(server.Client())

	token, err := p.exchangeToken(context.Background(), "test-oidc-token")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, "federated-access-token", token.AccessToken)
	assert.Equal(t, "Bearer", token.TokenType)
	assert.True(t, token.Expiry.After(time.Now()))
}

func TestExchangeToken_STSError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123456",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
		},
		stsURL: server.URL,
	}
	p.WithHTTPClient(server.Client())

	token, err := p.exchangeToken(context.Background(), "bad-token")
	require.Error(t, err)
	assert.Nil(t, token)
	assert.Contains(t, err.Error(), "STS error")
}

func TestExchangeToken_InvalidJSON(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	server.Listener = ln
	server.StartTLS()
	defer server.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123456",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
		},
		stsURL: server.URL,
	}
	p.WithHTTPClient(server.Client())

	token, err := p.exchangeToken(context.Background(), "test-token")
	require.Error(t, err)
	assert.Nil(t, token)
	assert.Contains(t, err.Error(), "decode STS response")
}

func TestImpersonateServiceAccount_Success(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
			Scopes:              []string{DefaultScope},
		},
		iamServiceFactory: func(_ context.Context, accessToken string) (gcp.IAMCredentialsService, error) {
			assert.Equal(t, "federated-token", accessToken)
			return &mockIAMService{
				resp: &iamcredentials.GenerateAccessTokenResponse{
					AccessToken: "impersonated-token",
					ExpireTime:  time.Now().Add(time.Hour).Format(time.RFC3339),
				},
			}, nil
		},
	}

	accessToken, expiry, err := p.impersonateServiceAccount(context.Background(), newOAuth2Token("federated-token"))
	require.NoError(t, err)
	assert.Equal(t, "impersonated-token", accessToken)
	assert.True(t, expiry.After(time.Now()))
}

func TestImpersonateServiceAccount_FactoryError(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		},
		iamServiceFactory: func(_ context.Context, _ string) (gcp.IAMCredentialsService, error) {
			return nil, fmt.Errorf("factory error")
		},
	}

	_, _, err := p.impersonateServiceAccount(context.Background(), newOAuth2Token("token"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create IAM credentials service")
}

func TestImpersonateServiceAccount_APIError(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ServiceAccountEmail: "sa@project.iam.gserviceaccount.com",
		},
		iamServiceFactory: func(_ context.Context, _ string) (gcp.IAMCredentialsService, error) {
			return &mockIAMService{
				err: fmt.Errorf("permission denied"),
			}, nil
		},
	}

	_, _, err := p.impersonateServiceAccount(context.Background(), newOAuth2Token("token"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// newOAuth2Token is a helper that creates a minimal oauth2.Token for testing.
func newOAuth2Token(accessToken string) *oauth2.Token {
	return &oauth2.Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}
}

func TestAuthenticate_FullFlow_WithoutImpersonation(t *testing.T) {
	// Start STS mock server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	stsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "federated-token", "expires_in": 3600, "token_type": "Bearer"}`))
	}))
	stsServer.Listener = ln
	stsServer.StartTLS()
	defer stsServer.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectID:                  "test-project",
			ProjectNumber:              "123456",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
			TokenSource: &types.WIFTokenSource{
				Type:                TokenSourceTypeEnvironment,
				EnvironmentVariable: "TEST_WIF_OIDC_TOKEN",
			},
		},
		stsURL: stsServer.URL,
	}
	p.WithHTTPClient(stsServer.Client())
	t.Setenv("TEST_WIF_OIDC_TOKEN", "my-oidc-token")

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "federated-token", gcpCreds.AccessToken)
	assert.Equal(t, "test-project", gcpCreds.ProjectID)
	assert.Empty(t, gcpCreds.ServiceAccountEmail)
}

func TestAuthenticate_FullFlow_WithImpersonation(t *testing.T) {
	// Start STS mock server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	stsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "federated-token", "expires_in": 3600, "token_type": "Bearer"}`))
	}))
	stsServer.Listener = ln
	stsServer.StartTLS()
	defer stsServer.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectID:                  "test-project",
			ProjectNumber:              "123456",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
			ServiceAccountEmail:        "sa@project.iam.gserviceaccount.com",
			TokenSource: &types.WIFTokenSource{
				Type:                TokenSourceTypeEnvironment,
				EnvironmentVariable: "TEST_WIF_OIDC_TOKEN_2",
			},
		},
		stsURL: stsServer.URL,
		iamServiceFactory: func(_ context.Context, accessToken string) (gcp.IAMCredentialsService, error) {
			assert.Equal(t, "federated-token", accessToken)
			return &mockIAMService{
				resp: &iamcredentials.GenerateAccessTokenResponse{
					AccessToken: "impersonated-sa-token",
					ExpireTime:  time.Now().Add(time.Hour).Format(time.RFC3339),
				},
			}, nil
		},
	}
	p.WithHTTPClient(stsServer.Client())
	t.Setenv("TEST_WIF_OIDC_TOKEN_2", "my-oidc-token")

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, creds)

	gcpCreds, ok := creds.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, "impersonated-sa-token", gcpCreds.AccessToken)
	assert.Equal(t, "test-project", gcpCreds.ProjectID)
	assert.Equal(t, "sa@project.iam.gserviceaccount.com", gcpCreds.ServiceAccountEmail)
}

func TestAuthenticate_ValidationFails(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			// Missing required fields.
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestAuthenticate_OIDCTokenFails(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
			// No token source configured.
		},
	}

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "get OIDC token")
}

func TestAuthenticate_ExchangeTokenFails(t *testing.T) {
	// Start STS mock that returns an error.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	stsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	stsServer.Listener = ln
	stsServer.StartTLS()
	defer stsServer.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
			TokenSource: &types.WIFTokenSource{
				Type:                TokenSourceTypeEnvironment,
				EnvironmentVariable: "TEST_WIF_EXCHANGE_FAIL",
			},
		},
		stsURL: stsServer.URL,
	}
	p.WithHTTPClient(stsServer.Client())
	t.Setenv("TEST_WIF_EXCHANGE_FAIL", "some-oidc-token")

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "token exchange")
}

func TestAuthenticate_ImpersonationFails(t *testing.T) {
	// Start STS mock that succeeds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("Skipping: unable to bind local listener: %v", err)
	}
	stsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "federated-token", "expires_in": 3600, "token_type": "Bearer"}`))
	}))
	stsServer.Listener = ln
	stsServer.StartTLS()
	defer stsServer.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:              "123",
			WorkloadIdentityPoolID:     "pool",
			WorkloadIdentityProviderID: "provider",
			ServiceAccountEmail:        "sa@project.iam.gserviceaccount.com",
			TokenSource: &types.WIFTokenSource{
				Type:                TokenSourceTypeEnvironment,
				EnvironmentVariable: "TEST_WIF_IMPERSONATE_FAIL",
			},
		},
		stsURL: stsServer.URL,
		iamServiceFactory: func(_ context.Context, _ string) (gcp.IAMCredentialsService, error) {
			return nil, fmt.Errorf("iam service creation failed")
		},
	}
	p.WithHTTPClient(stsServer.Client())
	t.Setenv("TEST_WIF_IMPERSONATE_FAIL", "some-oidc-token")

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Contains(t, err.Error(), "impersonate SA")
}

func TestGetStsURL_Default(t *testing.T) {
	p := &Provider{}
	assert.Equal(t, stsEndpoint, p.getStsURL())
}

func TestGetStsURL_Custom(t *testing.T) {
	p := &Provider{stsURL: "https://custom-sts.example.com/v1/token"}
	assert.Equal(t, "https://custom-sts.example.com/v1/token", p.getStsURL())
}
