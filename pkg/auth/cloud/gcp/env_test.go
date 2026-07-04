package gcp

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrepareEnvironment_ClearsGCPVars(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/creds.json")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	t.Setenv("CLOUDSDK_CONFIG", "/path/to/config")

	err := PrepareEnvironment()
	require.NoError(t, err)

	_, ok := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.False(t, ok)
	_, ok = os.LookupEnv("GOOGLE_CLOUD_PROJECT")
	assert.False(t, ok)
	_, ok = os.LookupEnv("CLOUDSDK_CONFIG")
	assert.False(t, ok)
}

func TestGetCurrentGCPEnvironment(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/creds.json")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	env := GetCurrentGCPEnvironment()
	assert.Equal(t, "/tmp/creds.json", env["GOOGLE_APPLICATION_CREDENTIALS"])
	assert.Equal(t, "test-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestGetEnvironmentVariables(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	env, err := GetEnvironmentVariables(testRealm, providerName, "env-identity")
	require.NoError(t, err)
	require.NotNil(t, env)
	assert.Equal(t, "config_atmos", env["CLOUDSDK_ACTIVE_CONFIG_NAME"])
	assert.NotEmpty(t, env["GOOGLE_APPLICATION_CREDENTIALS"])
	assert.NotEmpty(t, env["CLOUDSDK_CONFIG"])
	assert.Contains(t, env["GOOGLE_APPLICATION_CREDENTIALS"], "application_default_credentials.json")
	assert.Contains(t, env["GOOGLE_APPLICATION_CREDENTIALS"], providerName)
	assert.Contains(t, env["GOOGLE_APPLICATION_CREDENTIALS"], "env-identity")
}

func TestGetEnvironmentVariablesForIdentity_WithGCPAuth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	gcpAuth := &schema.GCPAuthContext{
		ProjectID:       "auth-project",
		Region:          "us-central1",
		ConfigDir:       "/custom/config/dir",
		CredentialsFile: "/custom/creds.json",
	}
	env, err := GetEnvironmentVariablesForIdentity(testRealm, providerName, "id", gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "/custom/creds.json", env["GOOGLE_APPLICATION_CREDENTIALS"])
	assert.Equal(t, "/custom/config/dir", env["CLOUDSDK_CONFIG"])
	assert.Equal(t, "auth-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "auth-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "auth-project", env["CLOUDSDK_CORE_PROJECT"])
	assert.Equal(t, "us-central1", env["GOOGLE_CLOUD_REGION"])
	assert.Equal(t, "us-central1", env["CLOUDSDK_COMPUTE_REGION"])
}

func TestRestoreEnvironment(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "original-project")
	saved := GetCurrentGCPEnvironment()
	require.NotEmpty(t, saved["GOOGLE_CLOUD_PROJECT"])

	// Clear and restore - use os.Unsetenv in defer block for manual restoration test.
	t.Cleanup(func() {
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	})
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	err := RestoreEnvironment(saved)
	require.NoError(t, err)
	assert.Equal(t, "original-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))
}

func TestSetEnvironmentVariables(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	err := SetEnvironmentVariables(ctx, testRealm, providerName, "setenv-identity")
	require.NoError(t, err)

	credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.NotEmpty(t, credsPath)
	assert.Contains(t, credsPath, providerName)
	assert.Contains(t, credsPath, "setenv-identity")
	assert.Contains(t, credsPath, "application_default_credentials.json")
	assert.NotEmpty(t, os.Getenv("CLOUDSDK_CONFIG"))
	assert.Equal(t, "config_atmos", os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"))

	// Clean up for other tests.
	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
}

func TestSetEnvironmentVariablesFromStackInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	// Test with stack info containing GCP auth context.
	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{
			GCP: &schema.GCPAuthContext{
				ProjectID: "stack-project",
				Region:    "eu-west1",
			},
		},
	}

	err := SetEnvironmentVariablesFromStackInfo(ctx, stackInfo, testRealm, providerName, "stack-identity")
	require.NoError(t, err)

	assert.Equal(t, "stack-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))
	assert.Equal(t, "eu-west1", os.Getenv("GOOGLE_CLOUD_REGION"))

	// Clean up for other tests.
	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
}

func TestSetEnvironmentVariablesFromStackInfo_NilStackInfo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()
	providerName := "gcp-adc"

	// nil stackInfo should not panic and should set file-based env vars.
	err := SetEnvironmentVariablesFromStackInfo(ctx, nil, testRealm, providerName, "nil-stack-identity")
	require.NoError(t, err)

	// Should still set file-based env vars.
	assert.NotEmpty(t, os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	assert.NotEmpty(t, os.Getenv("CLOUDSDK_CONFIG"))

	// Clean up for other tests.
	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
}

func TestProjectEnvVars_Empty(t *testing.T) {
	env := ProjectEnvVars("")
	assert.Empty(t, env)
}

func TestProjectEnvVars_WithProject(t *testing.T) {
	env := ProjectEnvVars("my-project")
	assert.Equal(t, "my-project", env["GOOGLE_CLOUD_PROJECT"])
	assert.Equal(t, "my-project", env["GCLOUD_PROJECT"])
	assert.Equal(t, "my-project", env["CLOUDSDK_CORE_PROJECT"])
}

func TestZoneEnvVars_Empty(t *testing.T) {
	env := ZoneEnvVars("")
	assert.Empty(t, env)
}

func TestZoneEnvVars_WithZone(t *testing.T) {
	env := ZoneEnvVars("us-central1-a")
	assert.Equal(t, "us-central1-a", env["GOOGLE_CLOUD_ZONE"])
	assert.Equal(t, "us-central1-a", env["CLOUDSDK_COMPUTE_ZONE"])
}

func TestGetEnvironmentVariablesForIdentity_WithAccessToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	// When access token is present, GOOGLE_OAUTH_ACCESS_TOKEN should be set
	// and GOOGLE_APPLICATION_CREDENTIALS should NOT be set.
	gcpAuth := &schema.GCPAuthContext{
		AccessToken: "ya29.access-token",
		ProjectID:   "token-project",
	}
	env, err := GetEnvironmentVariablesForIdentity(testRealm, providerName, "token-id", gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "ya29.access-token", env["GOOGLE_OAUTH_ACCESS_TOKEN"])
	_, hasADC := env["GOOGLE_APPLICATION_CREDENTIALS"]
	assert.False(t, hasADC, "GOOGLE_APPLICATION_CREDENTIALS should not be set when access token is available")
}

func TestGetEnvironmentVariablesForIdentity_WithLocation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	gcpAuth := &schema.GCPAuthContext{
		Location: "us-central1-a",
	}
	env, err := GetEnvironmentVariablesForIdentity(testRealm, providerName, "zone-id", gcpAuth)
	require.NoError(t, err)
	assert.Equal(t, "us-central1-a", env["GOOGLE_CLOUD_ZONE"])
	assert.Equal(t, "us-central1-a", env["CLOUDSDK_COMPUTE_ZONE"])
}
