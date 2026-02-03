package gcp_wif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

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
				ProjectNumber:          "123",
				WorkloadIdentityPool:   "pool",
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

	// Test missing env var.
	os.Unsetenv("TEST_OIDC_TOKEN")
	_, err := p.getTokenFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	// Test with env var set.
	os.Setenv("TEST_OIDC_TOKEN", "my-oidc-token")
	defer os.Unsetenv("TEST_OIDC_TOKEN")

	token, err := p.getTokenFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "my-oidc-token", token)
}

func TestGetTokenFromEnv_RequiresExplicitEnvVar(t *testing.T) {
	// Test that environment_variable must be explicitly specified.
	// This prevents users from accidentally using ACTIONS_ID_TOKEN_REQUEST_TOKEN
	// which is the request token, not the OIDC token itself.
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type: TokenSourceTypeEnvironment,
				// No EnvironmentVariable specified
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
	require.NoError(t, os.WriteFile(tokenFile, []byte("file-oidc-token\n"), 0600))

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-request-token", r.Header.Get("Authorization"))
		assert.Equal(t, "test-audience", r.URL.Query().Get("audience"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "url-oidc-token"}`))
	}))
	defer server.Close()

	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			TokenSource: &types.WIFTokenSource{
				Type:         TokenSourceTypeURL,
				URL:          server.URL,
				RequestToken: "test-request-token",
				Audience:     "test-audience",
			},
		},
	}

	token, err := p.getTokenFromURL(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "url-oidc-token", token)
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
			WorkloadIdentityProvider:  "legacy-provider",
		},
	}
	assert.Equal(t, "pool-id", p.poolID())
	assert.Equal(t, "provider-id", p.providerID())
}

func TestPoolIDAndProviderID_FallbackToLegacy(t *testing.T) {
	p := &Provider{
		spec: &types.GCPWorkloadIdentityFederationProviderSpec{
			ProjectNumber:          "123",
			WorkloadIdentityPool:   "legacy-pool",
			WorkloadIdentityProvider: "legacy-provider",
		},
	}
	assert.Equal(t, "legacy-pool", p.poolID())
	assert.Equal(t, "legacy-provider", p.providerID())
}
