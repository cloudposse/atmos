package gcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRealm = "test-realm"

func TestGetGCPBaseDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")

	base, err := GetGCPBaseDir()
	require.NoError(t, err)
	assert.Contains(t, base, "atmos")
	assert.Equal(t, filepath.Join(tmp, "atmos"), base)
}

func TestGetProviderDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetProviderDir(testRealm, providerName)
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetProviderDir_RequiresRealm(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetProviderDir("", "gcp-adc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "realm is required")
}

func TestGetProviderDir_InvalidName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetProviderDir(testRealm, "bad/name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestGetADCDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetADCDir(testRealm, providerName, "my-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "my-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetADCDir_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	_, err := GetADCDir(testRealm, providerName, "../id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path separators")
}

func TestGetADCFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := GetADCFilePath(testRealm, providerName, "dev")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ADCSubdir, "dev", CredentialsFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestGetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	dir, err := GetConfigDir(testRealm, providerName, "prod-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ConfigSubdir, "prod-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetPropertiesFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := GetPropertiesFilePath(testRealm, providerName, "test-id")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, providerName, ConfigSubdir, "test-id", PropertiesFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestWriteADCFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	content := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "ya29.test-token",
		TokenExpiry: "2025-12-31T23:59:59Z",
	}
	path, err := WriteADCFile(testRealm, providerName, "adc-identity", content)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.Contains(t, path, "application_default_credentials.json")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var parsed AuthorizedUserContent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "authorized_user", parsed.Type)
	assert.Equal(t, "ya29.test-token", parsed.AccessToken)
	assert.Equal(t, "2025-12-31T23:59:59Z", parsed.TokenExpiry)

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWriteADCFile_NilContent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := WriteADCFile(testRealm, "gcp-adc", "id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestWritePropertiesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := WritePropertiesFile(testRealm, providerName, "props-id", "my-project", "us-central1")
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "[core]")
	assert.Contains(t, content, "project = my-project")
	assert.Contains(t, content, "[compute]")
	assert.Contains(t, content, "region = us-central1")

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestWritePropertiesFile_EmptyProjectRegion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	path, err := WritePropertiesFile(testRealm, providerName, "empty-id", "", "")
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[core]")
	assert.Contains(t, string(data), "[compute]")
	assert.NotEmpty(t, path)
}

func TestWriteAccessTokenFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	expiry := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	path, err := WriteAccessTokenFile(testRealm, providerName, "token-id", "ya29.access-token", expiry)
	require.NoError(t, err)
	assert.NotEmpty(t, path)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := string(data)
	assert.Contains(t, lines, "ya29.access-token")
	assert.Contains(t, lines, "2025-06-15")
}

func TestCleanupIdentityFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	providerName := "gcp-adc"

	_, err := WriteADCFile(testRealm, providerName, "cleanup-id", &AuthorizedUserContent{Type: "authorized_user", AccessToken: "x"})
	require.NoError(t, err)
	_, err = WritePropertiesFile(testRealm, providerName, "cleanup-id", "p", "r")
	require.NoError(t, err)

	adcPath, _ := GetADCFilePath(testRealm, providerName, "cleanup-id")
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	err = CleanupIdentityFiles(testRealm, providerName, "cleanup-id")
	require.NoError(t, err)

	_, err = os.Stat(adcPath)
	require.True(t, os.IsNotExist(err))

	base, _ := GetGCPBaseDir()
	adcDir := filepath.Join(base, testRealm, GCPSubdir, providerName, ADCSubdir, "cleanup-id")
	configDir := filepath.Join(base, testRealm, GCPSubdir, providerName, ConfigSubdir, "cleanup-id")
	_, err = os.Stat(adcDir)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(configDir)
	require.True(t, os.IsNotExist(err))
}

func TestCleanupIdentityFiles_Nonexistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := CleanupIdentityFiles(testRealm, "gcp-adc", "nonexistent-identity")
	require.NoError(t, err)
}

func TestWriteAccessTokenFile_EmptyToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := WriteAccessTokenFile(testRealm, "gcp-adc", "empty-token-id", "", time.Time{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access token cannot be empty")
}

func TestWriteAccessTokenFile_ZeroExpiry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := WriteAccessTokenFile(testRealm, "gcp-adc", "zero-expiry-id", "ya29.token", time.Time{})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "ya29.token")
	// Zero expiry should not write a second line with timestamp.
	assert.Equal(t, "ya29.token\n", content)
}

func TestWritePropertiesFile_SpecialCharacters(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Test that ini.v1 properly handles special characters in values.
	path, err := WritePropertiesFile(testRealm, "gcp-adc", "special-id", "my-project-with-dashes_123", "us-east1")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "my-project-with-dashes_123")
	assert.Contains(t, content, "us-east1")
}

func TestWritePropertiesFile_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("File permission tests not reliable on Windows")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := WritePropertiesFile(testRealm, "gcp-adc", "perm-id", "proj", "region")
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteADCFile_Overwrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Write first version.
	content1 := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "first-token",
	}
	path1, err := WriteADCFile(testRealm, "gcp-adc", "overwrite-id", content1)
	require.NoError(t, err)

	// Write second version to same identity.
	content2 := &AuthorizedUserContent{
		Type:        "authorized_user",
		AccessToken: "second-token",
	}
	path2, err := WriteADCFile(testRealm, "gcp-adc", "overwrite-id", content2)
	require.NoError(t, err)
	assert.Equal(t, path1, path2)

	// Verify second version is written.
	data, err := os.ReadFile(path2)
	require.NoError(t, err)
	var parsed AuthorizedUserContent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "second-token", parsed.AccessToken)
}

func TestGetConfigDir_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetConfigDir(testRealm, "gcp-adc", "..")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be")
}

func TestGetConfigDir_EmptyIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := GetConfigDir(testRealm, "gcp-adc", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity name is required")
}

func TestGetAccessTokenFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := GetAccessTokenFilePath(testRealm, "gcp-adc", "token-path-id")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", testRealm, GCPSubdir, "gcp-adc", ADCSubdir, "token-path-id", AccessTokenFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestCleanupIdentityFiles_InvalidIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := CleanupIdentityFiles(testRealm, "gcp-adc", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identity name is required")
}

func TestValidatePathSegment(t *testing.T) {
	tests := []struct {
		name      string
		label     string
		value     string
		wantErr   bool
		errSubstr string
	}{
		{name: "valid segment", label: "test", value: "valid-name", wantErr: false},
		{name: "empty value", label: "test", value: "", wantErr: true, errSubstr: "is required"},
		{name: "dot segment", label: "test", value: ".", wantErr: true, errSubstr: "must not be"},
		{name: "dotdot segment", label: "test", value: "..", wantErr: true, errSubstr: "must not be"},
		{name: "forward slash", label: "test", value: "a/b", wantErr: true, errSubstr: "path separators"},
		{name: "backslash", label: "test", value: "a\\b", wantErr: true, errSubstr: "path separators"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathSegment(tt.label, tt.value)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
