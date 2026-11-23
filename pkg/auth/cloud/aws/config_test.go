package aws

// Tests that BuildAWSConfigFromCreds is isolated from the user's AWS
// environment (issue #3720, finding 3.8).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// TestBuildAWSConfigFromCreds_IgnoresUserAWSEnvironment verifies that a stale
// AWS_PROFILE, and settings in the user's shared config files, do not break or
// leak into the config built from Atmos credentials.
func TestBuildAWSConfigFromCreds_IgnoresUserAWSEnvironment(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config")
	credentialsFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(configFile, []byte("[default]\nregion = ap-southeast-2\nretry_mode = adaptive\n"), 0o600))
	require.NoError(t, os.WriteFile(credentialsFile, []byte("[default]\naws_access_key_id = AKIAUSERFILE\naws_secret_access_key = user-secret\n"), 0o600))

	// AWS_PROFILE names a profile that no longer exists in the user's files.
	t.Setenv("AWS_PROFILE", "stale-profile-that-does-not-exist")
	t.Setenv("AWS_CONFIG_FILE", configFile)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsFile)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAUSERENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "user-env-secret")

	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAATMOS",
		SecretAccessKey: "atmos-secret",
		SessionToken:    "atmos-token",
		Region:          "us-east-2",
	}

	cfg, err := BuildAWSConfigFromCreds(t.Context(), creds, "")
	require.NoError(t, err, "a stale AWS_PROFILE must not break Atmos-authenticated clients")
	assert.Equal(t, "us-east-2", cfg.Region)

	got, err := cfg.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "AKIAATMOS", got.AccessKeyID)
	assert.Equal(t, "atmos-token", got.SessionToken)

	// The user's environment is restored afterwards.
	assert.Equal(t, "stale-profile-that-does-not-exist", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, configFile, os.Getenv("AWS_CONFIG_FILE"))
}
