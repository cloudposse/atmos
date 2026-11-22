package gcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// gcloudActivePropertiesPath mirrors how gcloud resolves the properties file for
// the active named configuration: CLOUDSDK_ACTIVE_CONFIG_NAME=NAME selects
// $CLOUDSDK_CONFIG/configurations/config_NAME (see `gcloud topic configurations`).
// gcloud does not read $CLOUDSDK_CONFIG/properties for a named configuration.
func gcloudActivePropertiesPath(env map[string]string) string {
	return filepath.Join(env["CLOUDSDK_CONFIG"], "configurations", "config_"+env["CLOUDSDK_ACTIVE_CONFIG_NAME"])
}

// TestSetupFiles_PropertiesAreWhereGcloudReadsThem verifies the properties Atmos
// writes land in the file gcloud actually loads for the configuration Atmos selects.
func TestSetupFiles_PropertiesAreWhereGcloudReadsThem(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeADCClientCredentials(t)

	_, err := SetupFiles(context.Background(), testRealm, "gcp-adc", "gcloud-identity", &types.GCPCredentials{
		AccessToken: "ya29.test-token",
		TokenExpiry: time.Now().Add(time.Hour),
		ProjectID:   "test-project",
	})
	require.NoError(t, err)

	env, err := GetEnvironmentVariablesForIdentity(testRealm, "gcp-adc", "gcloud-identity", nil)
	require.NoError(t, err)

	data, err := os.ReadFile(gcloudActivePropertiesPath(env))
	require.NoError(t, err, "gcloud would find no properties for the active configuration")
	assert.Contains(t, string(data), "project = test-project")
}

// TestGetPropertiesFilePath_ConfigurationsDirFailure verifies a configurations path
// that cannot be created (it exists as a regular file) is reported, not ignored.
func TestGetPropertiesFilePath_ConfigurationsDirFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configDir, err := GetConfigDir(testRealm, "gcp-adc", "blocked-identity")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, ConfigurationsSubdir), []byte("not a dir"), 0o600))

	_, err = GetPropertiesFilePath(testRealm, "gcp-adc", "blocked-identity")
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)

	_, err = WritePropertiesFile(testRealm, "gcp-adc", "blocked-identity", "p1", "")
	require.ErrorIs(t, err, errUtils.ErrWritePropertiesFile)
}

// TestWritePropertiesFile_PathMatchesActiveConfig verifies the path WritePropertiesFile
// returns is the active-configuration file selected by the environment.
func TestWritePropertiesFile_PathMatchesActiveConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := WritePropertiesFile(testRealm, "gcp-adc", "gcloud-identity", "p1", "us-central1")
	require.NoError(t, err)

	env, err := GetEnvironmentVariablesForIdentity(testRealm, "gcp-adc", "gcloud-identity", nil)
	require.NoError(t, err)
	assert.Equal(t, gcloudActivePropertiesPath(env), path)
}
