package aws

import (
	"path/filepath"
	"testing"

	ini "gopkg.in/ini.v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetupFiles_WritesCredentialsAndConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret", SessionToken: "token", Region: "us-east-2"}
	err := SetupFiles("prov", "dev", creds)
	require.NoError(t, err)

	credPath := filepath.Join(tmp, ".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(tmp, ".aws", "atmos", "prov", "config")

	// Verify credentials file
	cfg, err := ini.Load(credPath)
	require.NoError(t, err)
	st, err := os.Stat(credPath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), st.Mode().Perm(), "credentials file should be 0600")
	sec := cfg.Section("dev")
	assert.Equal(t, "AKIA123", sec.Key("aws_access_key_id").String())
	assert.Equal(t, "secret", sec.Key("aws_secret_access_key").String())
	assert.Equal(t, "token", sec.Key("aws_session_token").String())

	// Verify config file
	cfg2, err := ini.Load(cfgPath)
	require.NoError(t, err)
	st2, err := os.Stat(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), st2.Mode().Perm(), "config file should be 0600")
	sec = cfg2.Section("profile dev")
	assert.Equal(t, "us-east-2", sec.Key("region").String())
}

func TestSetEnvironmentVariables_SetsStackEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	stack := &schema.ConfigAndStacksInfo{}
	err := SetEnvironmentVariables(stack, "prov", "dev")
	require.NoError(t, err)

	credPath := filepath.Join(".aws", "atmos", "prov", "credentials")
	cfgPath := filepath.Join(".aws", "atmos", "prov", "config")

	assert.Contains(t, stack.ComponentEnvSection["AWS_SHARED_CREDENTIALS_FILE"], credPath)
	assert.Contains(t, stack.ComponentEnvSection["AWS_CONFIG_FILE"], cfgPath)
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
}
