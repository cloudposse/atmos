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
	ctx := context.Background()
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/path/to/creds.json")
	os.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	os.Setenv("CLOUDSDK_CONFIG", "/path/to/config")
	defer func() {
		os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("CLOUDSDK_CONFIG")
	}()

	err := PrepareEnvironment(ctx, nil)
	require.NoError(t, err)

	_, ok := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.False(t, ok)
	_, ok = os.LookupEnv("GOOGLE_CLOUD_PROJECT")
	assert.False(t, ok)
	_, ok = os.LookupEnv("CLOUDSDK_CONFIG")
	assert.False(t, ok)
}

func TestGetCurrentGCPEnvironment(t *testing.T) {
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/creds.json")
	os.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")
	defer func() {
		os.Unsetenv("GOOGLE_APPLICATION_CREDENTIALS")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	}()

	env := GetCurrentGCPEnvironment()
	assert.Equal(t, "/tmp/creds.json", env["GOOGLE_APPLICATION_CREDENTIALS"])
	assert.Equal(t, "test-project", env["GOOGLE_CLOUD_PROJECT"])
}

func TestGetEnvironmentVariables(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	env, err := GetEnvironmentVariables(nil, "env-identity")
	require.NoError(t, err)
	require.NotNil(t, env)
	assert.Equal(t, "config_atmos", env["CLOUDSDK_ACTIVE_CONFIG_NAME"])
	assert.NotEmpty(t, env["GOOGLE_APPLICATION_CREDENTIALS"])
	assert.NotEmpty(t, env["CLOUDSDK_CONFIG"])
	assert.Contains(t, env["GOOGLE_APPLICATION_CREDENTIALS"], "application_default_credentials.json")
	assert.Contains(t, env["GOOGLE_APPLICATION_CREDENTIALS"], "env-identity")
}

func TestGetEnvironmentVariablesForIdentity_WithGCPAuth(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	gcpAuth := &schema.GCPAuthContext{
		ProjectID: "auth-project",
		Region:    "us-central1",
		ConfigDir: "/custom/config/dir",
		CredentialsFile: "/custom/creds.json",
	}
	env, err := GetEnvironmentVariablesForIdentity("id", gcpAuth)
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
	ctx := context.Background()
	os.Setenv("GOOGLE_CLOUD_PROJECT", "original-project")
	saved := GetCurrentGCPEnvironment()
	require.NotEmpty(t, saved["GOOGLE_CLOUD_PROJECT"])

	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	err := RestoreEnvironment(ctx, saved)
	require.NoError(t, err)
	assert.Equal(t, "original-project", os.Getenv("GOOGLE_CLOUD_PROJECT"))

	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
}

func TestSetEnvironmentVariables(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	ctx := context.Background()

	err := SetEnvironmentVariables(ctx, nil, "setenv-identity")
	require.NoError(t, err)

	credsPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	assert.NotEmpty(t, credsPath)
	assert.Contains(t, credsPath, "setenv-identity")
	assert.Contains(t, credsPath, "application_default_credentials.json")
	assert.NotEmpty(t, os.Getenv("CLOUDSDK_CONFIG"))
	assert.Equal(t, "config_atmos", os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"))

	// Clean up for other tests.
	for _, key := range GCPEnvironmentVariables {
		os.Unsetenv(key)
	}
}
