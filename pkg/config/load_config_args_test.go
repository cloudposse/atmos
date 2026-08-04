package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
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

// TestLoadConfigFromCLIArgs_InvalidEditionPin covers the applyEditionDefaults call this
// --config path added: this path skips the main LoadConfig flow's edition hook, so it
// applies (and validates) the pin itself. An unparseable `edition:` value must surface
// as an error here too, not silently unmarshal into an invalid AtmosConfig.
func TestLoadConfigFromCLIArgs_InvalidEditionPin(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")

	configContent := `
base_path: "."
edition: "not-a-date"
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configFile},
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

// TestLoadConfigFromCLIArgs_AppliesGitRootBasePath reproduces cloudposse/atmos#2863:
// loading config via --config with an empty base_path skipped git-root discovery,
// because loadConfigFromCLIArgs never called applyGitRootBasePath, unlike the main
// LoadConfig auto-discovery flow (load.go). This asserts the desired end state --
// BasePath resolved to the (mocked) git root -- so it fails before the fix lands
// and passes once loadConfigFromCLIArgs calls applyGitRootBasePath.
func TestLoadConfigFromCLIArgs_AppliesGitRootBasePath(t *testing.T) {
	// The fixture atmos.yaml lives in its own temp dir, separate from the process
	// cwd: hasLocalAtmosConfig only inspects the cwd, never the --config file's
	// directory, so the two must be kept apart to exercise the real code path.
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "atmos.yaml")

	configContent := `
base_path: ""
stacks:
  base_path: "stacks"
components:
  terraform:
    base_path: "components/terraform"
`
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))

	// cwd must be a separate, atmos-config-free directory so hasLocalAtmosConfig(cwd)
	// returns false and git-root discovery is not skipped.
	cwd := t.TempDir()
	t.Chdir(cwd)

	// Mock git-root discovery deterministically (see pkg/utils/git.go ProcessTagGitRoot,
	// which short-circuits to TEST_GIT_ROOT when set, bypassing real git detection).
	t.Setenv("TEST_GIT_ROOT", "/mock/git/repo/root")

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configFile},
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.NoError(t, err)

	// Desired end state: BasePath resolves to the git root, exactly like the main
	// LoadConfig auto-discovery path does for the same atmos.yaml content.
	assert.Equal(t, "/mock/git/repo/root", atmosConfig.BasePath)
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

func TestLoadConfigFromCLIArgs_WithCommandStepDefaultsInclude(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "atmos.yaml")
	sharedConfigFile := filepath.Join(tmpDir, "cast-defaults.yaml")

	configContent := fmt.Sprintf(`
base_path: "."
commands:
  - name: test-command
    steps:
      - type: cast
        name: demo
        command: atmos version
        mode: steps
        defaults:
          simulate: !include %s .simulate
        rate: 12ms
        steps:
          - type: simulate
            text: atmos version
`, sharedConfigFile)
	sharedConfigContent := `
simulate:
  mode: typed
  cursor: true
  prompt:
    text: '> '
    style: command
  rate: 35ms
`
	require.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o644))
	require.NoError(t, os.WriteFile(sharedConfigFile, []byte(sharedConfigContent), 0o644))

	v := viper.New()
	v.SetConfigType("yaml")

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configFile},
	}

	var atmosConfig schema.AtmosConfiguration
	err := loadConfigFromCLIArgs(v, configAndStacksInfo, &atmosConfig)
	require.NoError(t, err)

	require.Len(t, atmosConfig.Commands, 1)
	require.Len(t, atmosConfig.Commands[0].Steps, 1)
	castStep := atmosConfig.Commands[0].Steps[0]
	assert.Equal(t, schema.TaskTypeCast, castStep.Type)
	assert.Equal(t, "steps", castStep.Mode)
	assert.Equal(t, "12ms", castStep.Rate)
	require.NotNil(t, castStep.Defaults)
	require.NotNil(t, castStep.Defaults.Simulate)
	assert.Equal(t, "typed", castStep.Defaults.Simulate.Mode)
	require.NotNil(t, castStep.Defaults.Simulate.Cursor)
	assert.True(t, *castStep.Defaults.Simulate.Cursor)
	require.NotNil(t, castStep.Defaults.Simulate.Prompt)
	assert.Equal(t, "> ", castStep.Defaults.Simulate.Prompt.Text)
	assert.Equal(t, "command", castStep.Defaults.Simulate.Prompt.Style)
	assert.Equal(t, "35ms", castStep.Defaults.Simulate.Rate)
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
