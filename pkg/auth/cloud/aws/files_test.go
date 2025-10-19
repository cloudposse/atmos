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

func TestAWSFileManager_CleanupIdentity(t *testing.T) {
	tests := []struct {
		name               string
		setupCredentials   func(*AWSFileManager)
		providerName       string
		identityName       string
		verifyAfterCleanup func(*testing.T, *AWSFileManager)
	}{
		{
			name: "removes single identity section from credentials and config",
			setupCredentials: func(m *AWSFileManager) {
				creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret"}
				_ = m.WriteCredentials("test-provider", "identity1", creds)
				_ = m.WriteConfig("test-provider", "identity1", "us-east-1", "json")
			},
			providerName: "test-provider",
			identityName: "identity1",
			verifyAfterCleanup: func(t *testing.T, m *AWSFileManager) {
				// Files should be removed since no sections remain (only DEFAULT section left).
				_, err := os.Stat(m.GetCredentialsPath("test-provider"))
				assert.True(t, os.IsNotExist(err), "credentials file should be removed when empty")
				// Config file should also be removed.
				_, err = os.Stat(m.GetConfigPath("test-provider"))
				assert.True(t, os.IsNotExist(err), "config file should be removed when empty")
			},
		},
		{
			name: "preserves other identities when removing one",
			setupCredentials: func(m *AWSFileManager) {
				creds1 := &types.AWSCredentials{AccessKeyID: "AKIA1", SecretAccessKey: "secret1"}
				creds2 := &types.AWSCredentials{AccessKeyID: "AKIA2", SecretAccessKey: "secret2"}
				_ = m.WriteCredentials("test-provider", "identity1", creds1)
				_ = m.WriteCredentials("test-provider", "identity2", creds2)
				_ = m.WriteConfig("test-provider", "identity1", "us-east-1", "json")
				_ = m.WriteConfig("test-provider", "identity2", "us-west-2", "yaml")
			},
			providerName: "test-provider",
			identityName: "identity1",
			verifyAfterCleanup: func(t *testing.T, m *AWSFileManager) {
				// identity2 should still exist.
				credsPath := m.GetCredentialsPath("test-provider")
				cfg, err := ini.Load(credsPath)
				assert.NoError(t, err)
				assert.False(t, cfg.HasSection("identity1"), "identity1 should be removed")
				assert.True(t, cfg.HasSection("identity2"), "identity2 should remain")
				sec := cfg.Section("identity2")
				assert.Equal(t, "AKIA2", sec.Key("aws_access_key_id").String())

				// Config should also preserve identity2.
				configPath := m.GetConfigPath("test-provider")
				cfg, err = ini.Load(configPath)
				assert.NoError(t, err)
				assert.False(t, cfg.HasSection("profile identity1"))
				assert.True(t, cfg.HasSection("profile identity2"))
			},
		},
		{
			name: "handles non-existent identity gracefully",
			setupCredentials: func(m *AWSFileManager) {
				creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret"}
				_ = m.WriteCredentials("test-provider", "identity1", creds)
			},
			providerName: "test-provider",
			identityName: "nonexistent",
			verifyAfterCleanup: func(t *testing.T, m *AWSFileManager) {
				// identity1 should still exist.
				credsPath := m.GetCredentialsPath("test-provider")
				cfg, err := ini.Load(credsPath)
				assert.NoError(t, err)
				assert.True(t, cfg.HasSection("identity1"))
			},
		},
		{
			name: "handles non-existent files gracefully",
			setupCredentials: func(m *AWSFileManager) {
				// Don't create any files.
			},
			providerName: "test-provider",
			identityName: "identity1",
			verifyAfterCleanup: func(t *testing.T, m *AWSFileManager) {
				// No error should occur even though files don't exist.
			},
		},
		{
			name: "removes default identity using default section name",
			setupCredentials: func(m *AWSFileManager) {
				creds := &types.AWSCredentials{AccessKeyID: "AKIA123", SecretAccessKey: "secret"}
				_ = m.WriteCredentials("test-provider", "default", creds)
				_ = m.WriteConfig("test-provider", "default", "us-east-1", "json")
			},
			providerName: "test-provider",
			identityName: "default",
			verifyAfterCleanup: func(t *testing.T, m *AWSFileManager) {
				// Files should be removed since no sections remain.
				_, err := os.Stat(m.GetCredentialsPath("test-provider"))
				assert.True(t, os.IsNotExist(err), "credentials file should be removed")
				_, err = os.Stat(m.GetConfigPath("test-provider"))
				assert.True(t, os.IsNotExist(err), "config file should be removed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			m := &AWSFileManager{baseDir: tmp}

			tt.setupCredentials(m)

			err := m.CleanupIdentity(tt.providerName, tt.identityName)
			assert.NoError(t, err)

			tt.verifyAfterCleanup(t, m)
		})
	}
}

func TestAWSFileManager_RemoveIniSection(t *testing.T) {
	tests := []struct {
		name         string
		setupFile    func(string) string
		sectionName  string
		expectError  bool
		verifyResult func(*testing.T, string)
	}{
		{
			name: "removes section from multi-section file",
			setupFile: func(tmp string) string {
				filePath := filepath.Join(tmp, "test.ini")
				cfg := ini.Empty()
				sec1, _ := cfg.NewSection("section1")
				sec1.NewKey("key1", "value1")
				sec2, _ := cfg.NewSection("section2")
				sec2.NewKey("key2", "value2")
				_ = cfg.SaveTo(filePath)
				return filePath
			},
			sectionName: "section1",
			expectError: false,
			verifyResult: func(t *testing.T, filePath string) {
				cfg, err := ini.Load(filePath)
				assert.NoError(t, err)
				assert.False(t, cfg.HasSection("section1"))
				assert.True(t, cfg.HasSection("section2"))
			},
		},
		{
			name: "removes file when last section is removed",
			setupFile: func(tmp string) string {
				filePath := filepath.Join(tmp, "test.ini")
				cfg := ini.Empty()
				sec, _ := cfg.NewSection("only-section")
				sec.NewKey("key", "value")
				_ = cfg.SaveTo(filePath)
				return filePath
			},
			sectionName: "only-section",
			expectError: false,
			verifyResult: func(t *testing.T, filePath string) {
				_, err := os.Stat(filePath)
				assert.True(t, os.IsNotExist(err), "file should be removed")
			},
		},
		{
			name: "handles non-existent file gracefully",
			setupFile: func(tmp string) string {
				return filepath.Join(tmp, "nonexistent.ini")
			},
			sectionName: "section1",
			expectError: false,
			verifyResult: func(t *testing.T, filePath string) {
				_, err := os.Stat(filePath)
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name: "handles removing non-existent section",
			setupFile: func(tmp string) string {
				filePath := filepath.Join(tmp, "test.ini")
				cfg := ini.Empty()
				sec, _ := cfg.NewSection("section1")
				sec.NewKey("key1", "value1")
				_ = cfg.SaveTo(filePath)
				return filePath
			},
			sectionName: "nonexistent-section",
			expectError: false,
			verifyResult: func(t *testing.T, filePath string) {
				cfg, err := ini.Load(filePath)
				assert.NoError(t, err)
				assert.True(t, cfg.HasSection("section1"), "existing section should remain")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			m := &AWSFileManager{baseDir: tmp}

			filePath := tt.setupFile(tmp)

			err := m.removeIniSection(filePath, tt.sectionName)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			tt.verifyResult(t, filePath)
		})
	}
}
