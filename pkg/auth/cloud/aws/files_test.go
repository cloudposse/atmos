package aws

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ini "gopkg.in/ini.v1"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// TestMain disables homedir caching to prevent cached values from affecting test isolation.
func TestMain(m *testing.M) {
	homedir.DisableCache = true
	os.Exit(m.Run())
}

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

func TestAWSFileManager_DeleteIdentity(t *testing.T) {
	tests := []struct {
		name              string
		setupCredentials  func(*AWSFileManager)
		providerName      string
		identityName      string
		verifyAfterDelete func(*testing.T, *AWSFileManager)
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
			verifyAfterDelete: func(t *testing.T, m *AWSFileManager) {
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
			verifyAfterDelete: func(t *testing.T, m *AWSFileManager) {
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
			verifyAfterDelete: func(t *testing.T, m *AWSFileManager) {
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
			verifyAfterDelete: func(t *testing.T, m *AWSFileManager) {
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
			verifyAfterDelete: func(t *testing.T, m *AWSFileManager) {
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

			err := m.DeleteIdentity(context.Background(), tt.providerName, tt.identityName)
			assert.NoError(t, err)

			tt.verifyAfterDelete(t, m)
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

func TestAWSFileManager_GetBaseDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
	}{
		{
			name:    "returns configured base directory",
			baseDir: "/custom/path/aws",
		},
		{
			name:    "returns default base directory",
			baseDir: "~/.aws/atmos",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &AWSFileManager{baseDir: tt.baseDir}
			result := m.GetBaseDir()
			assert.Equal(t, tt.baseDir, result)
		})
	}
}

func TestAWSFileManager_GetDisplayPath(t *testing.T) {
	homeDir, err := homedir.Dir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		baseDir  string
		expected string
	}{
		{
			name:     "replaces home directory with tilde",
			baseDir:  filepath.Join(homeDir, ".aws", "atmos"),
			expected: filepath.ToSlash(filepath.Join("~", ".aws", "atmos")),
		},
		{
			name:     "keeps absolute path when not under home",
			baseDir:  "/opt/aws/atmos",
			expected: "/opt/aws/atmos",
		},
		{
			name:     "handles root directory",
			baseDir:  "/",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &AWSFileManager{baseDir: tt.baseDir}
			result := m.GetDisplayPath()
			// Normalize path separators for cross-platform compatibility.
			normalizedResult := filepath.ToSlash(result)
			assert.Equal(t, tt.expected, normalizedResult)
		})
	}
}

func TestNewAWSFileManager_LegacyPathWarning(t *testing.T) {
	// This test verifies that NewAWSFileManager detects legacy ~/.aws/atmos paths
	// and logs a warning. This is important for users migrating from pre-XDG versions.

	// Create a temp directory to act as home.
	tempHome := t.TempDir()
	legacyPath := filepath.Join(tempHome, ".aws", "atmos")

	// Create legacy directory structure.
	err := os.MkdirAll(legacyPath, 0o755)
	require.NoError(t, err)

	// Override home directory for this test.
	t.Setenv("HOME", tempHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tempHome)
	}

	// Set XDG config to use temp directory.
	xdgConfigDir := filepath.Join(tempHome, ".config")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigDir)

	// Create file manager (should trigger warning about legacy path).
	fm, err := NewAWSFileManager("")
	require.NoError(t, err)
	require.NotNil(t, fm)

	// Verify that new base directory is XDG-compliant, not legacy.
	assert.Contains(t, fm.baseDir, filepath.Join(xdgConfigDir, "atmos", "aws"),
		"New file manager should use XDG config directory")
	assert.NotContains(t, fm.baseDir, filepath.Join(".aws", "atmos"),
		"New file manager should not use legacy path")

	// Note: Actual warning log verification would require capturing log output,
	// which is beyond the scope of this unit test. The warning is logged by checkLegacyAWSAtmosPath.
	t.Logf("Legacy path created at: %s", legacyPath)
	t.Logf("New base directory: %s", fm.baseDir)
}

func TestNewAWSFileManager_NoLegacyPath(t *testing.T) {
	// This test verifies that no warning is logged when legacy path doesn't exist.

	// Create a temp directory to act as home.
	tempHome := t.TempDir()

	// Override home directory for this test.
	t.Setenv("HOME", tempHome)

	// Set XDG config to use temp directory.
	xdgConfigDir := filepath.Join(tempHome, ".config")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigDir)

	// Create file manager without legacy path (should not trigger warning).
	fm, err := NewAWSFileManager("")
	require.NoError(t, err)
	require.NotNil(t, fm)

	// Verify XDG-compliant path.
	assert.Contains(t, fm.baseDir, filepath.Join(xdgConfigDir, "atmos", "aws"))

	t.Logf("New base directory: %s", fm.baseDir)
}

// TestAWSFileManager_CustomBasePath verifies that provider-specific base_path configuration works.
func TestAWSFileManager_CustomBasePath(t *testing.T) {
	tests := []struct {
		name             string
		basePath         string
		expectedBasePath string
		setupEnv         func(*testing.T)
	}{
		{
			name:             "uses custom base_path from provider config",
			basePath:         "~/.aws/atmos",
			expectedBasePath: ".aws/atmos",
			setupEnv:         func(t *testing.T) {},
		},
		{
			name:             "uses absolute custom base_path",
			basePath:         "/tmp/custom-aws-creds",
			expectedBasePath: "/tmp/custom-aws-creds",
			setupEnv:         func(t *testing.T) {},
		},
		{
			name:             "expands tilde in base_path",
			basePath:         "~/custom/path",
			expectedBasePath: "custom/path",
			setupEnv:         func(t *testing.T) {},
		},
		{
			name:             "empty base_path uses XDG default",
			basePath:         "",
			expectedBasePath: filepath.Join(".config", "atmos", "aws"),
			setupEnv: func(t *testing.T) {
				homeDir, err := homedir.Dir()
				require.NoError(t, err)
				t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sandbox HOME to prevent tests from touching real user directories.
			fakeHome := t.TempDir()
			t.Setenv("HOME", fakeHome)
			if runtime.GOOS == "windows" {
				t.Setenv("USERPROFILE", fakeHome)
			}

			tt.setupEnv(t)

			fm, err := NewAWSFileManager(tt.basePath)
			require.NoError(t, err)
			require.NotNil(t, fm)

			// Normalize paths for cross-platform comparison (Windows uses backslashes).
			normalizedBaseDir := filepath.ToSlash(fm.baseDir)
			normalizedExpected := filepath.ToSlash(tt.expectedBasePath)

			// Verify the base directory contains the expected path.
			assert.Contains(t, normalizedBaseDir, normalizedExpected,
				"Base directory should contain expected path")

			t.Logf("Base path: %s → Resolved: %s", tt.basePath, fm.baseDir)
		})
	}
}

// TestAWSFileManager_BasePathCredentialIsolation verifies that different
// base_path configurations properly isolate credentials.
func TestAWSFileManager_BasePathCredentialIsolation(t *testing.T) {
	// Create two file managers with different base paths.
	basePath1 := t.TempDir()
	basePath2 := t.TempDir()

	fm1, err := NewAWSFileManager(basePath1)
	require.NoError(t, err)

	fm2, err := NewAWSFileManager(basePath2)
	require.NoError(t, err)

	// Write credentials to both managers with same provider/identity.
	creds1 := &types.AWSCredentials{
		AccessKeyID:     "AKIA_MANAGER1",
		SecretAccessKey: "secret1",
		SessionToken:    "token1",
	}

	creds2 := &types.AWSCredentials{
		AccessKeyID:     "AKIA_MANAGER2",
		SecretAccessKey: "secret2",
		SessionToken:    "token2",
	}

	providerName := "test-provider"
	identityName := "test-identity"

	err = fm1.WriteCredentials(providerName, identityName, creds1)
	require.NoError(t, err)

	err = fm2.WriteCredentials(providerName, identityName, creds2)
	require.NoError(t, err)

	// Verify credentials are isolated to their respective base paths.
	cfg1, err := ini.Load(fm1.GetCredentialsPath(providerName))
	require.NoError(t, err)
	sec1 := cfg1.Section(identityName)
	assert.Equal(t, "AKIA_MANAGER1", sec1.Key("aws_access_key_id").String())

	cfg2, err := ini.Load(fm2.GetCredentialsPath(providerName))
	require.NoError(t, err)
	sec2 := cfg2.Section(identityName)
	assert.Equal(t, "AKIA_MANAGER2", sec2.Key("aws_access_key_id").String())

	// Verify paths are different.
	assert.NotEqual(t, fm1.GetCredentialsPath(providerName), fm2.GetCredentialsPath(providerName),
		"Different base paths should produce different credential file paths")

	t.Logf("Manager 1 credentials: %s", fm1.GetCredentialsPath(providerName))
	t.Logf("Manager 2 credentials: %s", fm2.GetCredentialsPath(providerName))
}

// TestAWSFileManager_BasePathEnvironmentVariables verifies that environment
// variables point to the correct base_path location.
func TestAWSFileManager_BasePathEnvironmentVariables(t *testing.T) {
	customBasePath := t.TempDir()
	fm, err := NewAWSFileManager(customBasePath)
	require.NoError(t, err)

	providerName := "custom-sso"
	identityName := "dev-identity"

	envVars := fm.GetEnvironmentVariables(providerName, identityName)

	// Verify environment variables point to custom base path.
	expectedCredsPath := filepath.Join(customBasePath, providerName, "credentials")
	expectedConfigPath := filepath.Join(customBasePath, providerName, "config")

	assert.Equal(t, expectedCredsPath, envVars[0].Value, "AWS_SHARED_CREDENTIALS_FILE should use custom base_path")
	assert.Equal(t, expectedConfigPath, envVars[1].Value, "AWS_CONFIG_FILE should use custom base_path")
	assert.Equal(t, identityName, envVars[2].Value, "AWS_PROFILE should be identity name")

	t.Logf("Environment variables with custom base_path:")
	for _, env := range envVars {
		t.Logf("  %s=%s", env.Key, env.Value)
	}
}

// TestAWSFileManager_BasePathLegacyCompatibility verifies that setting
// base_path to legacy location works for migration scenarios.
func TestAWSFileManager_BasePathLegacyCompatibility(t *testing.T) {
	// Sandbox HOME to prevent tests from touching real user directories.
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", fakeHome)
	}

	// Simulate legacy path: ~/.aws/atmos.
	legacyBasePath := filepath.Join(fakeHome, ".aws", "atmos")

	// Create file manager with legacy base path.
	fm, err := NewAWSFileManager(legacyBasePath)
	require.NoError(t, err)

	// Write credentials.
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_LEGACY",
		SecretAccessKey: "legacy-secret",
	}

	providerName := "legacy-provider"
	identityName := "legacy-identity"

	err = fm.WriteCredentials(providerName, identityName, creds)
	require.NoError(t, err)

	// Verify credentials are written to legacy path.
	expectedPath := filepath.Join(legacyBasePath, providerName, "credentials")
	assert.Equal(t, expectedPath, fm.GetCredentialsPath(providerName))

	// Verify credentials file exists and contains correct data.
	cfg, err := ini.Load(expectedPath)
	require.NoError(t, err)
	sec := cfg.Section(identityName)
	assert.Equal(t, "AKIA_LEGACY", sec.Key("aws_access_key_id").String())

	// No manual cleanup needed - t.TempDir() automatically cleans up.

	t.Logf("Legacy credentials path: %s", expectedPath)
}

// TestAWSFileManager_BasePathInvalidPath tests error handling for invalid base paths.
func TestAWSFileManager_BasePathInvalidPath(t *testing.T) {
	// Test with path that cannot be expanded.
	invalidPath := "~nonexistentuser/path"

	_, err := NewAWSFileManager(invalidPath)
	assert.Error(t, err, "Should fail with invalid home directory expansion")
	assert.Contains(t, err.Error(), "invalid base_path")
}

func TestAWSFileManager_GetCachePath(t *testing.T) {
	tests := []struct {
		name              string
		setupEnv          func(*testing.T) string // returns temp home dir
		expectedPathParts []string                // path parts that should be in result
		shouldBeEmpty     bool
	}{
		{
			name: "uses XDG_CACHE_HOME when set",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				xdgCache := filepath.Join(tempHome, "custom-cache")
				t.Setenv("XDG_CACHE_HOME", xdgCache)
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{"custom-cache", "aws", "sso", "cache"},
		},
		{
			name: "expands tilde in XDG_CACHE_HOME",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				t.Setenv("XDG_CACHE_HOME", "~/.cache")
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{".cache", "aws", "sso", "cache"},
		},
		{
			name: "falls back to default when XDG_CACHE_HOME empty",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				t.Setenv("XDG_CACHE_HOME", "")
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{".aws", "sso", "cache"},
		},
		{
			name: "falls back to default when XDG_CACHE_HOME not set",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				// Don't set XDG_CACHE_HOME at all
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{".aws", "sso", "cache"},
		},
		{
			name: "handles XDG_CACHE_HOME with trailing slash",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				xdgCache := filepath.Join(tempHome, "cache") + string(filepath.Separator)
				t.Setenv("XDG_CACHE_HOME", xdgCache)
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{"cache", "aws"},
		},
		{
			name: "handles XDG_CACHE_HOME with whitespace",
			setupEnv: func(t *testing.T) string {
				tempHome := t.TempDir()
				xdgCache := filepath.Join(tempHome, "cache")
				t.Setenv("XDG_CACHE_HOME", "  "+xdgCache+"  ")
				t.Setenv("HOME", tempHome)
				if runtime.GOOS == "windows" {
					t.Setenv("USERPROFILE", tempHome)
				}
				return tempHome
			},
			expectedPathParts: []string{"cache", "aws", "sso", "cache"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempHome := tt.setupEnv(t)

			m := &AWSFileManager{baseDir: tempHome}
			result := m.GetCachePath()

			if tt.shouldBeEmpty {
				assert.Empty(t, result, "should return empty string when home dir unavailable")
			} else {
				assert.NotEmpty(t, result, "should return non-empty cache path")
				// Normalize to forward slashes for cross-platform comparison
				normalizedResult := filepath.ToSlash(result)
				for _, part := range tt.expectedPathParts {
					assert.Contains(t, normalizedResult, part,
						"cache path should contain %s", part)
				}
			}

			t.Logf("Cache path: %s", result)
		})
	}
}

func TestAWSFileManager_GetCachePath_AbsolutePaths(t *testing.T) {
	// Test that cache paths are absolute, not relative
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tempHome)
	}

	tests := []struct {
		name     string
		xdgCache string
	}{
		{
			name:     "with absolute XDG path",
			xdgCache: filepath.Join(tempHome, ".cache"),
		},
		{
			name:     "with tilde XDG path",
			xdgCache: "~/.cache",
		},
		{
			name:     "without XDG (default)",
			xdgCache: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.xdgCache != "" {
				t.Setenv("XDG_CACHE_HOME", tt.xdgCache)
			}

			m := &AWSFileManager{baseDir: tempHome}
			result := m.GetCachePath()

			assert.NotEmpty(t, result)
			assert.True(t, filepath.IsAbs(result),
				"cache path should be absolute, got: %s", result)

			t.Logf("Cache path: %s (absolute: %v)", result, filepath.IsAbs(result))
		})
	}
}

func TestAWSFileManager_GetCachePath_CrossPlatform(t *testing.T) {
	// Verify cache paths work correctly across different platforms
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tempHome)
	}

	m := &AWSFileManager{baseDir: tempHome}
	cachePath := m.GetCachePath()

	assert.NotEmpty(t, cachePath)

	// Verify path separators are correct for the platform
	expectedSeparator := string(filepath.Separator)
	assert.Contains(t, cachePath, expectedSeparator,
		"cache path should use platform-specific separators")

	// Verify we can actually create the directory
	err := os.MkdirAll(cachePath, 0o755)
	assert.NoError(t, err, "should be able to create cache directory")

	// Clean up
	defer os.RemoveAll(cachePath)

	t.Logf("Platform: %s", runtime.GOOS)
	t.Logf("Cache path: %s", cachePath)
	t.Logf("Path separator: %s", expectedSeparator)
}
