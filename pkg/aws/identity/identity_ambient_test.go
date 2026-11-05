package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfigWithAmbientAuth preserves SDK profile/file discovery when an
// authenticated ambient identity publishes a region-only context (issue #3465).
func TestLoadConfigWithAmbientAuth(t *testing.T) {
	for _, source := range []string{"profile", "environment"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			configPath, credentialsPath := filepath.Join(root, "config"), filepath.Join(root, "credentials")
			require.NoError(t, os.WriteFile(configPath, []byte("[profile selected]\nregion = eu-west-1\n"), 0o600))
			require.NoError(t, os.WriteFile(credentialsPath, []byte("[selected]\naws_access_key_id = profile-access\naws_secret_access_key = profile-secret\naws_session_token = profile-token\n"), 0o600))
			for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN"} {
				t.Setenv(key, "")
			}
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			t.Setenv("AWS_CONFIG_FILE", configPath)
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", credentialsPath)
			t.Setenv("AWS_PROFILE", "selected")
			wantKey, wantToken := "profile-access", "profile-token"
			if source == "environment" {
				t.Setenv("AWS_ACCESS_KEY_ID", "environment-access")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "environment-secret")
				t.Setenv("AWS_SESSION_TOKEN", "environment-token")
				wantKey, wantToken = "environment-access", "environment-token"
			}
			for _, region := range []string{"", "ap-southeast-2"} {
				cfg, err := LoadConfigWithAuth(t.Context(), "", "", 0, &schema.AWSAuthContext{Region: region})
				require.NoError(t, err)
				creds, err := cfg.Credentials.Retrieve(t.Context())
				require.NoError(t, err)
				assert.Equal(t, wantKey, creds.AccessKeyID)
				assert.Equal(t, wantToken, creds.SessionToken)
				wantRegion := region
				if wantRegion == "" {
					wantRegion = "eu-west-1"
				}
				assert.Equal(t, wantRegion, cfg.Region)
			}
		})
	}
}

// TestLoadConfigWithManagedAuth verifies explicit managed files and profile still
// override ambient discovery after empty overrides become optional.
func TestLoadConfigWithManagedAuth(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_REGION", "AWS_DEFAULT_REGION", "AWS_DEFAULT_PROFILE", "AWS_WEB_IDENTITY_TOKEN_FILE", "AWS_ROLE_ARN"} {
		t.Setenv(key, "")
	}
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_PROFILE", "missing-ambient-profile")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(root, "absent-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(root, "absent-credentials"))
	configPath, credentialsPath := filepath.Join(root, "managed-config"), filepath.Join(root, "managed-credentials")
	require.NoError(t, os.WriteFile(configPath, []byte("[profile managed]\nregion = eu-west-1\n"), 0o600))
	require.NoError(t, os.WriteFile(credentialsPath, []byte("[managed]\naws_access_key_id = managed-access\naws_secret_access_key = managed-secret\n"), 0o600))
	cfg, err := LoadConfigWithAuth(t.Context(), "us-east-2", "", 0, &schema.AWSAuthContext{
		CredentialsFile: credentialsPath, ConfigFile: configPath, Profile: "managed", Region: "ap-southeast-2",
	})
	require.NoError(t, err)
	creds, err := cfg.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "managed-access", creds.AccessKeyID)
	assert.Equal(t, "us-east-2", cfg.Region, "explicit caller region must retain precedence")
}
