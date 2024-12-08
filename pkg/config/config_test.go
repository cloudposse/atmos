package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Config loads successfully from ATMOS_CLI_CONFIG_PATH when env var is set
func TestInitCliConfigLoadsFromAtmosCliConfigPath(t *testing.T) {
	// Setup test directory and config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "atmos.yaml")

	// Create test config content
	configContent := []byte(`
components:
  terraform:
    base_path: terraform
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_pattern: "{stage}"
`)
	err := os.WriteFile(configPath, configContent, 0644)
	require.NoError(t, err)

	// Set env var to temp config path
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	// Create test input
	configInfo := schema.ConfigAndStacksInfo{
		Stack: "test-stack",
	}

	// Call function under test
	cfg, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// Assert results
	require.True(t, cfg.Initialized)
	require.Equal(t, "terraform", cfg.Components.Terraform.BasePath)
}

// Empty or invalid ATMOS_CLI_CONFIG_PATH environment variable
func TestInitCliConfigWithInvalidEnvPath(t *testing.T) {
	// Set env var to non-existent path
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "/non/existent/path")

	// Create test input
	configInfo := schema.ConfigAndStacksInfo{
		Stack: "test-stack",
	}

	// Call function under test
	_, err := InitCliConfig(configInfo, false)

	// Assert error is returned
	require.Error(t, err)
	require.Contains(t, err.Error(), "config not found in ATMOS_CLI_CONFIG_PATH")
}

// Config loads from default locations when ATMOS_CLI_CONFIG_PATH is not set
func TestConfigLoadsFromDefaultLocations(t *testing.T) {
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	// Setup temporary directory and default config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "atmos.yaml")
	configContent := []byte(`
components:
  helmfile:
    use_eks: true
  terraform:
    append_user_agent: Atmos/1.0.0 (Cloud Posse; +https://atmos.tools)
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_pattern: "{stage}"
`)
	err := os.WriteFile(configPath, configContent, 0644)
	require.NoError(t, err)

	// Change working directory to temporary directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	cliConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)
	require.True(t, cliConfig.Initialized)
}

// Imports from atmos.d directory are processed automatically when no explicit imports defined
func TestImportsFromAtmosDProcessedAutomatically(t *testing.T) {
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	// Setup temporary directory and atmos.d
	tmpDir := t.TempDir()
	atmosDPath := filepath.Join(tmpDir, "atmos.d")
	err := os.Mkdir(atmosDPath, 0755)
	require.NoError(t, err)

	// Create a sample import file
	importFilePath := filepath.Join(atmosDPath, "sample.yaml")
	importContent := []byte(`
imports:
  - some/import/path.yaml
`)
	err = os.WriteFile(importFilePath, importContent, 0644)
	require.NoError(t, err)

	// Change working directory to temporary directory
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	cliConfig, err := InitCliConfig(configAndStacksInfo, false)
	require.NoError(t, err)

	if len(cliConfig.Import) == 0 {
		t.Fatalf("Expected imports to be processed from atmos.d directory")
	}
}
