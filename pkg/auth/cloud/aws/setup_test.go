package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ini "gopkg.in/ini.v1"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetupFiles_WritesCredentialsAndConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-2"}
	err := SetupFiles("prov", "dev", creds, "")
	require.NoError(t, err)

	credPath := filepath.Join(tmp, ".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(tmp, ".aws", "atmos", "prov", "config")

	// Verify credentials file.
	cfg, err := ini.Load(credPath)
	require.NoError(t, err)
	_, err = os.Stat(credPath)
	require.NoError(t, err)
	sec := cfg.Section("dev")
	assert.Equal(t, "AKIA123", sec.Key("aws_access_key_id").String())
	assert.Equal(t, "secret", sec.Key("aws_secret_access_key").String())
	assert.Equal(t, "token", sec.Key("aws_session_token").String())

	// Verify config file.
	cfg2, err := ini.Load(cfgPath)
	require.NoError(t, err)
	_, err = os.Stat(cfgPath)
	require.NoError(t, err)
	sec = cfg2.Section("profile dev")
	assert.Equal(t, "us-east-2", sec.Key("region").String())
}

func TestSetEnvironmentVariables_SetsStackEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	homedir.Reset() // Clear homedir cache to pick up the test HOME

	// Create auth context with AWS credentials.
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			CredentialsFile: filepath.Join(tmp, ".aws", "atmos", "prov", "credentials"),
			ConfigFile:      filepath.Join(tmp, ".aws", "atmos", "prov", "config"),
			Profile:         "dev",
		},
	}

	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(authContext, stack)
	require.NoError(t, err)

	credPath := filepath.Join(".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(".aws", "atmos", "prov", "config")

	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], credPath)
	assert.Contains(t, stack.ComponentEnvSection["AWS_CONFIG_FILE"], cfgPath)
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}
