package gcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func writeADCClientCredentials(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "application_default_credentials.json")
	payload := map[string]string{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
	return path
}

func TestSetupFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	writeADCClientCredentials(t)
	ctx := context.Background()
	providerName := "gcp-adc"

	creds := &types.GCPCredentials{
		AccessToken: "ya29.test-token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
		ProjectID:   "test-project",
	}
	paths, err := SetupFiles(ctx, testRealm, providerName, "setup-identity", creds)
	require.NoError(t, err)
	require.Len(t, paths, 3)

	adcPath := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "setup-identity", CredentialsFileName)
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	propsPath := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ConfigSubdir, "setup-identity", PropertiesFileName)
	data, err := os.ReadFile(propsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "project = test-project")
}

func TestSetupFiles_NilCreds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	_, err := SetupFiles(ctx, testRealm, providerName, "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestSetAuthContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	authContext := &schema.AuthContext{}
	creds := &types.GCPCredentials{
		AccessToken:         "token",
		TokenExpiry:         time.Now().Add(1 * time.Hour),
		ProjectID:           "proj-123",
		ServiceAccountEmail: "sa@proj.iam.gserviceaccount.com",
	}

	err := SetAuthContext(authContext, testRealm, providerName, "auth-id", creds)
	require.NoError(t, err)
	require.NotNil(t, authContext.GCP)
	assert.Equal(t, "proj-123", authContext.GCP.ProjectID)
	assert.Equal(t, "sa@proj.iam.gserviceaccount.com", authContext.GCP.ServiceAccountEmail)
	assert.Equal(t, "token", authContext.GCP.AccessToken)
	assert.NotEmpty(t, authContext.GCP.CredentialsFile)
	assert.NotEmpty(t, authContext.GCP.ConfigDir)
	assert.Contains(t, authContext.GCP.CredentialsFile, "application_default_credentials.json")
	assert.Contains(t, authContext.GCP.CredentialsFile, "auth-id")
}

func TestSetAuthContext_NilAuthContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	creds := &types.GCPCredentials{AccessToken: "x"}
	err := SetAuthContext(nil, testRealm, providerName, "id", creds)
	require.NoError(t, err)
}

func TestSetAuthContext_NilCreds(t *testing.T) {
	authContext := &schema.AuthContext{}
	err := SetAuthContext(authContext, testRealm, "gcp-adc", "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestSetup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	// Use ATMOS_GCP_ADC_CLIENT_SECRET because Setup calls PrepareEnvironment
	// which clears GOOGLE_APPLICATION_CREDENTIALS before SetupFiles reads it.
	t.Setenv("ATMOS_GCP_ADC_CLIENT_SECRET", "test-client-secret")
	ctx := context.Background()
	providerName := "gcp-adc"

	t.Setenv("GOOGLE_CLOUD_PROJECT", "old-project")

	creds := &types.GCPCredentials{
		AccessToken: "ya29.setup-token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
		ProjectID:   "new-project",
	}
	err := Setup(ctx, testRealm, providerName, "setup-full-identity", creds)
	require.NoError(t, err)

	assert.Equal(t, "new-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))
	// When we have an access token, we use GOOGLE_OAUTH_ACCESS_TOKEN instead of
	// GOOGLE_APPLICATION_CREDENTIALS (to avoid refresh token requirement in ADC file).
	assert.Equal(t, "ya29.setup-token", os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"))
	// GOOGLE_APPLICATION_CREDENTIALS should NOT be set when we have an access token.
	assert.Empty(t, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	// ADC file should still be written (for future use or tools that need it).
	adcPath := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "setup-full-identity", CredentialsFileName)
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	for _, key := range GCPEnvironmentVariables {
		t.Setenv(key, "")
	}
}

func TestSetupFiles_NoADCSecretAndNoADCFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(tmp, "missing.json"))

	ctx := context.Background()
	providerName := "gcp-adc"
	creds := &types.GCPCredentials{
		AccessToken: "token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
		ProjectID:   "test-project",
	}

	_, err := SetupFiles(ctx, testRealm, providerName, "setup-identity", creds)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "application-default login")
}

func TestCleanup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	writeADCClientCredentials(t)
	ctx := context.Background()
	providerName := "gcp-adc"

	creds := &types.GCPCredentials{AccessToken: "x", TokenExpiry: time.Now().Add(1 * time.Hour)}
	_, err := SetupFiles(ctx, testRealm, providerName, "cleanup-identity", creds)
	require.NoError(t, err)

	adcPath, _ := GetADCFilePath(testRealm, providerName, "cleanup-identity")
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	err = Cleanup(ctx, testRealm, providerName, "cleanup-identity")
	require.NoError(t, err)

	_, err = os.Stat(adcPath)
	require.True(t, os.IsNotExist(err))
}

func TestLoadCredentialsFromFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	content := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.loaded-token",
		TokenExpiry: "2026-01-15T12:00:00Z",
	}
	_, err := WriteADCFile(testRealm, providerName, "load-id", content)
	require.NoError(t, err)

	creds, err := LoadCredentialsFromFiles(ctx, testRealm, providerName, "load-id")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "ya29.loaded-token", creds.AccessToken)
	assert.False(t, creds.TokenExpiry.IsZero())
}

func TestLoadCredentialsFromFiles_NoFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	creds, err := LoadCredentialsFromFiles(ctx, testRealm, providerName, "nonexistent-load-id")
	require.NoError(t, err)
	assert.Nil(t, creds)
}

func TestCredentialsExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	exists, err := CredentialsExist(ctx, testRealm, providerName, "nonexistent-exist-id")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = WriteADCFile(testRealm, providerName, "exist-id", &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "token",
		TokenExpiry: time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)

	exists, err = CredentialsExist(ctx, testRealm, providerName, "exist-id")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCredentialsExist_Expired(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	_, err := WriteADCFile(testRealm, providerName, "expired-id", &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "token",
		TokenExpiry: "2020-01-01T00:00:00Z",
	})
	require.NoError(t, err)

	exists, err := CredentialsExist(ctx, testRealm, providerName, "expired-id")
	require.NoError(t, err)
	assert.False(t, exists)
}
