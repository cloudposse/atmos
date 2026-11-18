package s3test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// poisonedConfig is a developer's shared config whose default profile assumes a role, the setup that
// reaches the real STS service from a test that does not isolate it.
const poisonedConfig = "[default]\nrole_arn = arn:aws:iam::123456789012:role/developer\nsource_profile = base\nregion = eu-west-1\n\n" +
	"[profile base]\naws_access_key_id = AKIAPOISONED\naws_secret_access_key = poisoned\n"

func TestIsolateAWSConfig_IgnoresTheMachinesAWSSetup(t *testing.T) {
	ambient := t.TempDir()
	poisoned := filepath.Join(ambient, "config")
	require.NoError(t, os.WriteFile(poisoned, []byte(poisonedConfig), 0o600))
	t.Setenv("AWS_CONFIG_FILE", poisoned)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", poisoned)
	t.Setenv("AWS_PROFILE", "developer")
	t.Setenv("AWS_DEFAULT_PROFILE", "developer")
	t.Setenv("AWS_ROLE_ARN", "arn:aws:iam::123456789012:role/ambient")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", filepath.Join(ambient, "missing-token"))
	t.Setenv("AWS_ENDPOINT_URL", "http://ambient.invalid")

	dir := IsolateAWSConfig(t)

	for _, name := range ambientAWSVariables {
		assert.Empty(t, os.Getenv(name), name)
	}
	assert.Equal(t, filepath.Join(dir, "config"), os.Getenv("AWS_CONFIG_FILE"))
	assert.Equal(t, filepath.Join(dir, "credentials"), os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
	assert.Equal(t, "true", os.Getenv("AWS_EC2_METADATA_DISABLED"))
	for _, name := range []string{"config", "credentials"} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Zero(t, info.Size(), "the isolated %s file is empty", name)
	}

	// The default chain loads without the profile, the role, or the endpoint of the machine, and
	// the credentials it resolves are the ones the test sets.
	// The region is the one thing the helper leaves to the test, so this test pins it too.
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	loaded, err := config.LoadDefaultConfig(t.Context())
	require.NoError(t, err)
	assert.Empty(t, loaded.Region, "the machine's region is not merged in")
	credentials, err := loaded.Credentials.Retrieve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "test-key", credentials.AccessKeyID)
}
