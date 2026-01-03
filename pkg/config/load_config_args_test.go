package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadConfigFromCLIArgs_WithConfigFiles(t *testing.T) {
	// Create a temporary directory with a test config file.
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create a minimal config file.
	configContent := `
base_path: "."
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configFile},
	}

	var atmosConfig schema.AtmosConfiguration
	err = loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.NoError(t, err)

	assert.Equal(t, ".", atmosConfig.BasePath)
	assert.Equal(t, "stacks", atmosConfig.Stacks.BasePath)
}

func TestLoadConfigFromCLIArgs_WithConfigDirs(t *testing.T) {
	// Create a temporary directory with a test config file.
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create a minimal config file.
	configContent := `
base_path: "/test/base"
logs:
  level: Debug
`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{tmpDir},
	}

	var atmosConfig schema.AtmosConfiguration
	err = loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.NoError(t, err)

	assert.Equal(t, "/test/base", atmosConfig.BasePath)
	assert.Equal(t, "Debug", atmosConfig.Logs.Level)
}

func TestLoadConfigFromCLIArgs_NoConfigFound(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		// No config files or dirs specified.
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no config files found from command line arguments")
}

func TestLoadConfigFromCLIArgs_InvalidConfigFile(t *testing.T) {
	// Create a temporary directory with a non-existent config file path.
	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{"/non/existent/path/atmos.yaml"},
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.Error(t, err)
}

func TestLoadConfigFromCLIArgs_InvalidConfigDir(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigDirsFromArg: []string{"/non/existent/directory"},
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.Error(t, err)
}

func TestLoadConfigFromCLIArgs_WithCommands(t *testing.T) {
	// Create a temporary directory with a config file containing commands.
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	// Create a config file with commands that have steps (Tasks).
	configContent := `
base_path: "."
commands:
  - name: test-command
    description: "A test command"
    steps:
      - "echo hello"
      - name: structured-step
        command: "echo world"
        timeout: 30s
`
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configFile},
	}

	var atmosConfig schema.AtmosConfiguration
	err = loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.NoError(t, err)

	// Verify commands were parsed correctly.
	require.Len(t, atmosConfig.Commands, 1)
	assert.Equal(t, "test-command", atmosConfig.Commands[0].Name)

	// Verify steps (Tasks) were parsed with the decode hook.
	require.Len(t, atmosConfig.Commands[0].Steps, 2)
	assert.Equal(t, "echo hello", atmosConfig.Commands[0].Steps[0].Command)
	assert.Equal(t, "shell", atmosConfig.Commands[0].Steps[0].Type)
	assert.Equal(t, "structured-step", atmosConfig.Commands[0].Steps[1].Name)
	assert.Equal(t, "echo world", atmosConfig.Commands[0].Steps[1].Command)
}

func TestValidatedIsFiles_EmptyPath(t *testing.T) {
	err := validatedIsFiles([]string{""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config requires a non-empty file path")
}

func TestValidatedIsFiles_NonExistent(t *testing.T) {
	err := validatedIsFiles([]string{"/non/existent/file.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestValidatedIsFiles_IsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	err := validatedIsFiles([]string{tmpDir})
	require.Error(t, err)
}

func TestValidatedIsFiles_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(tmpFile, []byte("test: value"), 0o644)
	require.NoError(t, err)

	err = validatedIsFiles([]string{tmpFile})
	require.NoError(t, err)
}

func TestValidatedIsDirs_EmptyPath(t *testing.T) {
	err := validatedIsDirs([]string{""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--config-path requires a non-empty directory path")
}

func TestValidatedIsDirs_NonExistent(t *testing.T) {
	err := validatedIsDirs([]string{"/non/existent/directory"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestValidatedIsDirs_IsFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")
	err := os.WriteFile(tmpFile, []byte("test: value"), 0o644)
	require.NoError(t, err)

	err = validatedIsDirs([]string{tmpFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires a directory but found a file")
}

func TestValidatedIsDirs_ValidDir(t *testing.T) {
	tmpDir := t.TempDir()
	err := validatedIsDirs([]string{tmpDir})
	require.NoError(t, err)
}

func TestConnectPaths_SinglePath(t *testing.T) {
	result := connectPaths([]string{"/path/one"})
	assert.Equal(t, "/path/one", result)
}

func TestConnectPaths_MultiplePaths(t *testing.T) {
	result := connectPaths([]string{"/path/one", "/path/two", "/path/three"})
	assert.Equal(t, "/path/one;/path/two;/path/three;", result)
}

func TestConnectPaths_WithEmptyPaths(t *testing.T) {
	result := connectPaths([]string{"/path/one", "", "/path/two"})
	assert.Equal(t, "/path/one;/path/two;", result)
}

func TestConnectPaths_AllEmpty(t *testing.T) {
	result := connectPaths([]string{"", "", ""})
	assert.Equal(t, "", result)
}
