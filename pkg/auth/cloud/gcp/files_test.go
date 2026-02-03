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

func TestGetGCPBaseDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")

	base, err := GetGCPBaseDir()
	require.NoError(t, err)
	assert.Contains(t, base, "atmos")
	assert.Contains(t, base, GCPSubdir)
	assert.Equal(t, filepath.Join(tmp, "atmos", GCPSubdir), base)
}

func TestGetADCDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := GetADCDir("my-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", GCPSubdir, ADCSubdir, "my-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetADCFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := GetADCFilePath("dev")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", GCPSubdir, ADCSubdir, "dev", CredentialsFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestGetConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := GetConfigDir("prod-identity")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", GCPSubdir, ConfigSubdir, "prod-identity")
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(dir))
	_, err = os.Stat(dir)
	require.NoError(t, err)
}

func TestGetPropertiesFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := GetPropertiesFilePath("test-id")
	require.NoError(t, err)
	expected := filepath.Join(tmp, "atmos", GCPSubdir, ConfigSubdir, "test-id", PropertiesFileName)
	assert.Equal(t, filepath.ToSlash(expected), filepath.ToSlash(path))
}

func TestWriteADCFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	content := &ADCFileContent{
		Type:        "authorized_user",
		AccessToken: "ya29.test-token",
		TokenExpiry: "2025-12-31T23:59:59Z",
	}
	path, err := WriteADCFile("adc-identity", content)
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

	_, err := WriteADCFile("id", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestWritePropertiesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := WritePropertiesFile("props-id", "my-project", "us-central1")
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

	path, err := WritePropertiesFile("empty-id", "", "")
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

	expiry := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	path, err := WriteAccessTokenFile("token-id", "ya29.access-token", expiry)
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

	_, err := WriteADCFile("cleanup-id", &ADCFileContent{Type: "authorized_user", AccessToken: "x"})
	require.NoError(t, err)
	_, err = WritePropertiesFile("cleanup-id", "p", "r")
	require.NoError(t, err)

	adcPath, _ := GetADCFilePath("cleanup-id")
	_, err = os.Stat(adcPath)
	require.NoError(t, err)

	err = CleanupIdentityFiles("cleanup-id")
	require.NoError(t, err)

	_, err = os.Stat(adcPath)
	require.True(t, os.IsNotExist(err))

	base, _ := GetGCPBaseDir()
	adcDir := filepath.Join(base, ADCSubdir, "cleanup-id")
	configDir := filepath.Join(base, ConfigSubdir, "cleanup-id")
	_, err = os.Stat(adcDir)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(configDir)
	require.True(t, os.IsNotExist(err))
}

func TestCleanupIdentityFiles_Nonexistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := CleanupIdentityFiles("nonexistent-identity")
	require.NoError(t, err)
}
