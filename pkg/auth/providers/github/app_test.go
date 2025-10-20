package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
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

func generateTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey
}

func writePrivateKeyToFile(t *testing.T, key *rsa.PrivateKey, path string) {
	t.Helper()
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(key)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})
	err := os.WriteFile(path, privateKeyPEM, 0600)
	require.NoError(t, err)
}

func TestNewAppProvider(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	tests := []struct {
		name        string
		provName    string
		config      *schema.Provider
		expectError bool
		errorType   error
	}{
		{
			name:     "valid configuration with private_key_path",
			provName: "github-app",
			config: &schema.Provider{
				Kind: KindApp,
				Spec: map[string]interface{}{
					"app_id":          "12345",
					"installation_id": "67890",
					"private_key_path": keyPath,
				},
			},
			expectError: false,
		},
		{
			name:     "valid with permissions and repositories",
			provName: "github-app",
			config: &schema.Provider{
				Kind: KindApp,
				Spec: map[string]interface{}{
					"app_id":          "12345",
					"installation_id": "67890",
					"private_key_path": keyPath,
					"permissions": map[string]interface{}{
						"contents": "read",
						"metadata": "read",
					},
					"repositories": []interface{}{"repo1", "repo2"},
				},
			},
			expectError: false,
		},
		{
			name:        "nil config",
			provName:    "github-app",
			config:      nil,
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:        "empty provider name",
			provName:    "",
			config:      &schema.Provider{Kind: KindApp},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:     "missing app_id",
			provName: "github-app",
			config: &schema.Provider{
				Kind: KindApp,
				Spec: map[string]interface{}{
					"installation_id": "67890",
					"private_key_path": keyPath,
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:     "missing installation_id",
			provName: "github-app",
			config: &schema.Provider{
				Kind: KindApp,
				Spec: map[string]interface{}{
					"app_id":          "12345",
					"private_key_path": keyPath,
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
		{
			name:     "missing private key",
			provName: "github-app",
			config: &schema.Provider{
				Kind: KindApp,
				Spec: map[string]interface{}{
					"app_id":          "12345",
					"installation_id": "67890",
				},
			},
			expectError: true,
			errorType:   errUtils.ErrInvalidProviderConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewAppProvider(tt.provName, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
				assert.Equal(t, tt.provName, provider.Name())
				assert.Equal(t, KindApp, provider.Kind())
			}
		})
	}
}

func TestAppProvider_LoadPrivateKey_FromEnv(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	t.Setenv("GITHUB_APP_PRIVATE_KEY", string(privateKeyPEM))

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_env":  "GITHUB_APP_PRIVATE_KEY",
		},
	}

	provider, err := NewAppProvider("github-app", config)

	require.NoError(t, err)
	assert.NotNil(t, provider)
}

func TestAppProvider_Authenticate(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	// Create mock HTTP server for GitHub API.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/app/installations/67890/access_tokens")
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))

		response := map[string]interface{}{
			"token":      "ghs_installation_token",
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
		},
	}

	provider, err := NewAppProvider("github-app", config)
	require.NoError(t, err)

	// Override httpClient to use test server.
	appProv := provider.(*appProvider)
	appProv.httpClient = &http.Client{
		Transport: &roundTripperFunc{
			fn: func(req *http.Request) (*http.Response, error) {
				// Rewrite URL to point to test server.
				req.URL.Scheme = "http"
				req.URL.Host = server.URL[7:] // Remove "http://".
				return http.DefaultTransport.RoundTrip(req)
			},
		},
	}

	creds, err := provider.Authenticate(context.Background())

	require.NoError(t, err)
	require.NotNil(t, creds)

	githubCreds, ok := creds.(*types.GitHubAppCredentials)
	require.True(t, ok)
	assert.Equal(t, "ghs_installation_token", githubCreds.Token)
	assert.Equal(t, "12345", githubCreds.AppID)
	assert.Equal(t, "67890", githubCreds.InstallationID)
	assert.Equal(t, "github-app", githubCreds.Provider)
}

func TestAppProvider_Authenticate_WithPermissions(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request body contains permissions.
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		permissions, ok := reqBody["permissions"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "read", permissions["contents"])

		response := map[string]interface{}{
			"token":      "ghs_installation_token",
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
			"permissions": map[string]interface{}{
				"contents": "read",
				"metadata": "read",
			},
			"repositories": []interface{}{"repo1"},
		},
	}

	provider, err := NewAppProvider("github-app", config)
	require.NoError(t, err)

	appProv := provider.(*appProvider)
	appProv.httpClient = &http.Client{
		Transport: &roundTripperFunc{
			fn: func(req *http.Request) (*http.Response, error) {
				req.URL.Scheme = "http"
				req.URL.Host = server.URL[7:]
				return http.DefaultTransport.RoundTrip(req)
			},
		},
	}

	creds, err := provider.Authenticate(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, creds)
}

func TestAppProvider_Validate(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
		},
	}

	provider, err := NewAppProvider("github-app", config)
	require.NoError(t, err)

	err = provider.Validate()
	assert.NoError(t, err)
}

func TestAppProvider_Environment(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
		},
	}

	provider, err := NewAppProvider("github-app", config)
	require.NoError(t, err)

	env, err := provider.Environment()
	assert.NoError(t, err)
	assert.Empty(t, env)
}

func TestGitHubAppCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name       string
		expiration time.Time
		expected   bool
	}{
		{
			name:       "not expired",
			expiration: time.Now().Add(1 * time.Hour),
			expected:   false,
		},
		{
			name:       "expired",
			expiration: time.Now().Add(-1 * time.Hour),
			expected:   true,
		},
		{
			name:       "about to expire (within 5 min)",
			expiration: time.Now().Add(3 * time.Minute),
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &types.GitHubAppCredentials{
				Token:      "test-token",
				Expiration: tt.expiration,
			}

			assert.Equal(t, tt.expected, creds.IsExpired())
		})
	}
}

func TestGitHubAppCredentials_BuildWhoamiInfo(t *testing.T) {
	creds := &types.GitHubAppCredentials{
		Token:          "test-token",
		AppID:          "12345",
		InstallationID: "67890",
		Provider:       "github-app",
	}

	info := &types.WhoamiInfo{
		Environment: make(map[string]string),
	}

	creds.BuildWhoamiInfo(info)

	assert.Equal(t, "test-token", info.Environment["GITHUB_TOKEN"])
	assert.Equal(t, "test-token", info.Environment["GH_TOKEN"])
	assert.Equal(t, "12345", info.Environment["GITHUB_APP_ID"])
	assert.Equal(t, "67890", info.Environment["GITHUB_INSTALLATION_ID"])
}

func TestLoadPrivateKey_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.pem")
	err := os.WriteFile(keyPath, []byte("not a valid PEM"), 0600)
	require.NoError(t, err)

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
		},
	}

	provider, err := NewAppProvider("github-app", config)

	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "failed to parse PEM block")
}

func TestAppProvider_PreAuthenticate(t *testing.T) {
	privateKey := generateTestPrivateKey(t)
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "key.pem")
	writePrivateKeyToFile(t, privateKey, keyPath)

	config := &schema.Provider{
		Kind: KindApp,
		Spec: map[string]interface{}{
			"app_id":          "12345",
			"installation_id": "67890",
			"private_key_path": keyPath,
		},
	}

	provider, err := NewAppProvider("github-app", config)
	require.NoError(t, err)

	err = provider.PreAuthenticate(nil)
	assert.NoError(t, err)
}

// roundTripperFunc is a helper type for mocking HTTP round trips.
type roundTripperFunc struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f *roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.fn(req)
}
