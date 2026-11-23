package gcp_service_account

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	gcpCloud "github.com/cloudposse/atmos/pkg/auth/cloud/gcp"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema/type fields these tests rely on.
var (
	_ = schema.IdentityVia{Provider: "gcp-adc"}
	_ = types.GCPServiceAccountIdentityPrincipal{ServiceAccountEmail: "", ProjectID: ""}
)

func newPrepareEnvIdentity(name, projectID string) *Identity {
	return &Identity{
		name:  name,
		realm: "test-realm",
		principal: &types.GCPServiceAccountIdentityPrincipal{
			ServiceAccountEmail: "deployer@email-project.iam.gserviceaccount.com",
			ProjectID:           projectID,
		},
		config: &schema.Identity{Via: &schema.IdentityVia{Provider: "gcp-adc"}},
	}
}

// writeIdentityADC persists an ADC file for the identity the same way PostAuthenticate does.
func writeIdentityADC(t *testing.T, identityName, token string, expiry time.Time) {
	t.Helper()
	_, err := gcpCloud.WriteADCFile("test-realm", "gcp-adc", identityName, &gcpCloud.AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: token,
		TokenExpiry: expiry.UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
}

func TestPrepareEnvironment_WithStoredCredentials(t *testing.T) {
	tests := []struct {
		name        string
		projectID   string
		wantProject string
	}{
		{name: "principal project wins", projectID: "principal-project", wantProject: "principal-project"},
		{name: "project derived from service account email", projectID: "", wantProject: "email-project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			id := newPrepareEnvIdentity("sa-identity", tt.projectID)
			writeIdentityADC(t, "sa-identity", "ya29.stored-token", time.Now().Add(time.Hour))

			input := map[string]string{"PATH": "/usr/bin"}
			env, err := id.PrepareEnvironment(context.Background(), input)
			require.NoError(t, err)

			// Token comes from the identity's own realm-scoped ADC file.
			assert.Equal(t, "ya29.stored-token", env["GOOGLE_OAUTH_ACCESS_TOKEN"])
			// With an access token, the authorized_user ADC file (no refresh token) must not be
			// advertised via GOOGLE_APPLICATION_CREDENTIALS.
			assert.NotContains(t, env, "GOOGLE_APPLICATION_CREDENTIALS")
			assert.Equal(t, tt.wantProject, env["GOOGLE_CLOUD_PROJECT"])
			assert.Equal(t, tt.wantProject, env["CLOUDSDK_CORE_PROJECT"])
			assert.Equal(t, "/usr/bin", env["PATH"])

			// The gcloud config dir is isolated per realm, provider, and identity.
			wantConfigDir, err := gcpCloud.GetConfigDir("test-realm", "gcp-adc", "sa-identity")
			require.NoError(t, err)
			assert.Equal(t, wantConfigDir, env["CLOUDSDK_CONFIG"])
			assert.True(t, filepath.IsAbs(env["CLOUDSDK_CONFIG"]))

			// The input map must not be mutated.
			assert.Equal(t, map[string]string{"PATH": "/usr/bin"}, input)
		})
	}
}

func TestPrepareEnvironment_IdentitiesDoNotShareTokens(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeIdentityADC(t, "identity-a", "token-a", time.Now().Add(time.Hour))
	writeIdentityADC(t, "identity-b", "token-b", time.Now().Add(time.Hour))

	envA, err := newPrepareEnvIdentity("identity-a", "p").PrepareEnvironment(context.Background(), nil)
	require.NoError(t, err)
	envB, err := newPrepareEnvIdentity("identity-b", "p").PrepareEnvironment(context.Background(), nil)
	require.NoError(t, err)

	assert.Equal(t, "token-a", envA["GOOGLE_OAUTH_ACCESS_TOKEN"])
	assert.Equal(t, "token-b", envB["GOOGLE_OAUTH_ACCESS_TOKEN"])
	assert.NotEqual(t, envA["CLOUDSDK_CONFIG"], envB["CLOUDSDK_CONFIG"])
}

func TestPrepareEnvironment_ExpiredStoredCredentials(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	id := newPrepareEnvIdentity("expired-identity", "p")
	writeIdentityADC(t, "expired-identity", "ya29.expired", time.Now().Add(-time.Minute))

	env, err := id.PrepareEnvironment(context.Background(), map[string]string{})
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Nil(t, env)
	assert.Contains(t, err.Error(), "expired-identity")
}

// TestPrepareEnvironment_ScrubsCallerGCPEnvironment verifies the caller's own GCP
// credentials and location settings are not passed through next to the Atmos token:
// GOOGLE_APPLICATION_CREDENTIALS (or an impersonation/credential override) would make
// tools authenticate as someone else, and CLOUDSDK_COMPUTE_* would mix the caller's
// region/zone into the identity's project.
func TestPrepareEnvironment_ScrubsCallerGCPEnvironment(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	id := newPrepareEnvIdentity("sa-identity", "principal-project")
	writeIdentityADC(t, "sa-identity", "ya29.stored-token", time.Now().Add(time.Hour))

	input := map[string]string{
		"PATH":                                      "/usr/bin",
		"GOOGLE_APPLICATION_CREDENTIALS":            "/home/me/personal-key.json",
		"CLOUDSDK_COMPUTE_REGION":                   "caller-region",
		"CLOUDSDK_COMPUTE_ZONE":                     "caller-zone",
		"GOOGLE_CLOUD_REGION":                       "caller-region",
		"GOOGLE_CLOUD_ZONE":                         "caller-zone",
		"GOOGLE_CLOUD_PROJECT":                      "caller-project",
		"GOOGLE_CREDENTIALS":                        "{\"type\":\"service_account\"}",
		"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT":        "other@caller.iam.gserviceaccount.com",
		"CLOUDSDK_AUTH_ACCESS_TOKEN_FILE":           "/home/me/token",
		"CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT": "other@caller.iam.gserviceaccount.com",
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE":    "/home/me/override.json",
		"CLOUDSDK_CORE_ACCOUNT":                     "me@example.com",
	}
	env, err := id.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	assert.Equal(t, "ya29.stored-token", env["GOOGLE_OAUTH_ACCESS_TOKEN"])
	assert.Equal(t, "principal-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "/usr/bin", env["PATH"], "unrelated variables are kept")
	for _, key := range []string{
		"GOOGLE_APPLICATION_CREDENTIALS",
		"CLOUDSDK_COMPUTE_REGION",
		"CLOUDSDK_COMPUTE_ZONE",
		"GOOGLE_CLOUD_REGION",
		"GOOGLE_CLOUD_ZONE",
		"GOOGLE_CREDENTIALS",
		"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
		"CLOUDSDK_AUTH_ACCESS_TOKEN_FILE",
		"CLOUDSDK_AUTH_IMPERSONATE_SERVICE_ACCOUNT",
		"CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE",
		"CLOUDSDK_CORE_ACCOUNT",
	} {
		assert.NotContains(t, env, key, "caller's %s must not leak into the identity environment", key)
	}
	assert.Equal(t, "/home/me/personal-key.json", input["GOOGLE_APPLICATION_CREDENTIALS"], "the input map must not be mutated")
}
