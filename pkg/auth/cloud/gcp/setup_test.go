package gcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetupFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_GCP_ADC_CLIENT_SECRET", "test-secret")
	ctx := context.Background()
	providerName := "gcp-adc"

	creds := &types.GCPCredentials{
		AccessToken: "ya29.test-token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
		ProjectID:   "test-project",
	}
	paths, err := SetupFiles(ctx, nil, providerName, "setup-identity", creds)
	require.NoError(t, err)
	require.Len(t, paths, 3)

	adcPath := filepath.Join(tmp, "atmos", GCPSubdir, providerName, ADCSubdir, "setup-identity", CredentialsFileName)
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	propsPath := filepath.Join(tmp, "atmos", GCPSubdir, providerName, ConfigSubdir, "setup-identity", PropertiesFileName)
	data, err := os.ReadFile(propsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "project = test-project")
}

func TestSetupFiles_NilCreds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	_, err := SetupFiles(ctx, nil, providerName, "id", nil)
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

	err := SetAuthContext(authContext, providerName, "auth-id", creds)
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
	err := SetAuthContext(nil, providerName, "id", creds)
	require.NoError(t, err)
}

func TestSetAuthContext_NilCreds(t *testing.T) {
	authContext := &schema.AuthContext{}
	err := SetAuthContext(authContext, "gcp-adc", "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestSetup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_GCP_ADC_CLIENT_SECRET", "test-secret")
	ctx := context.Background()
	providerName := "gcp-adc"

	t.Setenv("GOOGLE_CLOUD_PROJECT", "old-project")

	creds := &types.GCPCredentials{
		AccessToken: "ya29.setup-token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
		ProjectID:   "new-project",
	}
	err := Setup(ctx, nil, providerName, "setup-full-identity", creds)
	require.NoError(t, err)

	assert.Equal(t, "new-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))
	// When we have an access token, we use GOOGLE_OAUTH_ACCESS_TOKEN instead of
	// GOOGLE_APPLICATION_CREDENTIALS (to avoid refresh token requirement in ADC file).
	assert.Equal(t, "ya29.setup-token", os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"))
	// GOOGLE_APPLICATION_CREDENTIALS should NOT be set when we have an access token.
	assert.Empty(t, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))

	// ADC file should still be written (for future use or tools that need it).
	adcPath := filepath.Join(tmp, "atmos", GCPSubdir, providerName, ADCSubdir, "setup-full-identity", CredentialsFileName)
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	for _, key := range GCPEnvironmentVariables {
		t.Setenv(key, "")
	}
}

func TestCleanup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_GCP_ADC_CLIENT_SECRET", "test-secret")
	ctx := context.Background()
	providerName := "gcp-adc"

	creds := &types.GCPCredentials{AccessToken: "x", TokenExpiry: time.Now().Add(1 * time.Hour)}
	_, err := SetupFiles(ctx, nil, providerName, "cleanup-identity", creds)
	require.NoError(t, err)

	adcPath, _ := GetADCFilePath(providerName, "cleanup-identity")
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	err = Cleanup(ctx, nil, providerName, "cleanup-identity")
	require.NoError(t, err)

	_, err = os.Stat(adcPath)
	require.True(t, os.IsNotExist(err))
}

func TestLoadCredentialsFromFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	content := &ADCFileContent{
		Type:        "authorized_user",
		AccessToken: "ya29.loaded-token",
		TokenExpiry: "2026-01-15T12:00:00Z",
	}
	_, err := WriteADCFile(providerName, "load-id", content)
	require.NoError(t, err)

	creds, err := LoadCredentialsFromFiles(ctx, nil, providerName, "load-id")
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

	creds, err := LoadCredentialsFromFiles(ctx, nil, providerName, "nonexistent-load-id")
	require.NoError(t, err)
	assert.Nil(t, creds)
}

func TestCredentialsExist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	exists, err := CredentialsExist(ctx, nil, providerName, "nonexistent-exist-id")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = WriteADCFile(providerName, "exist-id", &ADCFileContent{
		Type:        "authorized_user",
		AccessToken: "token",
		TokenExpiry: time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)

	exists, err = CredentialsExist(ctx, nil, providerName, "exist-id")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCredentialsExist_Expired(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	_, err := WriteADCFile(providerName, "expired-id", &ADCFileContent{
		Type:        "authorized_user",
		AccessToken: "token",
		TokenExpiry: "2020-01-01T00:00:00Z",
	})
	require.NoError(t, err)

	exists, err := CredentialsExist(ctx, nil, providerName, "expired-id")
	require.NoError(t, err)
	assert.False(t, exists)
}
