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

	content := &ADCFileContent{
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
	var parsed ADCFileContent
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

	_, err := WriteADCFile(testRealm, providerName, "cleanup-id", &ADCFileContent{Type: "authorized_user", AccessToken: "x"})
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
