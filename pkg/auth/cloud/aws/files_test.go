package aws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	ini "gopkg.in/ini.v1"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestAWSFileManager_WriteCredentials(t *testing.T) {
	tmp := t.TempDir()
	m := &AWSFileManager{baseDir: tmp}

	creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret", SessionToken: "token"}
	err := m.WriteCredentials("prov", "dev", creds)
	assert.NoError(t, err)

	path := m.GetCredentialsPath("prov")
	cfg, err := ini.Load(path)
	assert.NoError(t, err)
	sec := cfg.Section("dev")
	assert.Equal(t, "AKIA123", sec.Key("aws_access_key_id").String())
	assert.Equal(t, "secret", sec.Key("aws_secret_access_key").String())
	assert.Equal(t, "token", sec.Key("aws_session_token").String())

	// Overwrite without session token ensures key removal.
	creds.SessionToken = ""
	err = m.WriteCredentials("prov", "dev", creds)
	assert.NoError(t, err)
	cfg, err = ini.Load(path)
	assert.NoError(t, err)
	sec = cfg.Section("dev")
	_, err = sec.GetKey("aws_session_token")
	assert.Error(t, err) // key removed.
}

func TestAWSFileManager_WriteConfig(t *testing.T) {
	tmp := t.TempDir()
	m := &AWSFileManager{baseDir: tmp}

	// Non-default profile.
	err := m.WriteConfig("prov", "dev", "us-east-2", "json")
	assert.NoError(t, err)
	cfg, err := ini.Load(m.GetConfigPath("prov"))
	assert.NoError(t, err)
	sec := cfg.Section("profile dev")
	assert.Equal(t, "us-east-2", sec.Key("region").String())
	assert.Equal(t, "json", sec.Key("output").String())

	// Default profile uses "default" section.
	err = m.WriteConfig("prov", "default", "us-west-1", "")
	assert.NoError(t, err)
	cfg, err = ini.Load(m.GetConfigPath("prov"))
	assert.NoError(t, err)
	sec = cfg.Section("default")
	assert.Equal(t, "us-west-1", sec.Key("region").String())
	// output should be removed when empty.
	_, err = sec.GetKey("output")
	assert.Error(t, err)

	// Clear keys if empty values provided.
	err = m.WriteConfig("prov", "dev", "", "")
	assert.NoError(t, err)
	cfg, err = ini.Load(m.GetConfigPath("prov"))
	assert.NoError(t, err)
	sec = cfg.Section("profile dev")
	_, err = sec.GetKey("region")
	assert.Error(t, err)
	_, err = sec.GetKey("output")
	assert.Error(t, err)
}

func TestAWSFileManager_PathsEnvCleanup(t *testing.T) {
	tmp := t.TempDir()
	m := &AWSFileManager{baseDir: tmp}
	credsPath := m.GetCredentialsPath("prov")
	cfgPath := m.GetConfigPath("prov")
	assert.Equal(t, filepath.Join(tmp, "prov", "credentials"), credsPath)
	assert.Equal(t, filepath.Join(tmp, "prov", "config"), cfgPath)

	// Ensure env variables are produced.
	env := m.GetEnvironmentVariables("prov", "dev")
	assert.Equal(t, 3, len(env))

	// Create and cleanup provider dir.
	_ = os.MkdirAll(filepath.Dir(credsPath), 0o755)
	f, err := os.Create(credsPath)
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
	err = m.Cleanup("prov")
	assert.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(tmp, "prov"))
	assert.True(t, os.IsNotExist(statErr))
}
