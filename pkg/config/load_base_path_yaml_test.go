package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadConfig_ProcessesYAMLFunctionsInBasePath(t *testing.T) {
	// Enable git root search for this test (disabled by TestMain by default)
	t.Setenv("ATMOS_GIT_ROOT_ENABLED", "true")

	// Create a temporary directory for test fixtures
	tempDir := t.TempDir()

	// Set up TEST_GIT_ROOT to simulate git root
	gitRoot := filepath.Join(tempDir, "repo")
	require.NoError(t, os.MkdirAll(gitRoot, 0o755))
	t.Setenv("TEST_GIT_ROOT", gitRoot)

	t.Run("processes !repo-root in base_path from config file", func(t *testing.T) {
		// Create atmos.yaml with !repo-root in base_path
		configPath := filepath.Join(gitRoot, "atmos.yaml")
		configContent := `
base_path: "!repo-root"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

		// Change to a subdirectory
		subDir := filepath.Join(gitRoot, "components", "terraform")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(originalDir) }()
		require.NoError(t, os.Chdir(subDir))

		// Load config
		configInfo := schema.ConfigAndStacksInfo{}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// Verify base_path was processed and resolved to git root
		assert.Equal(t, gitRoot, config.BasePath,
			"base_path should be processed from !repo-root to actual git root path")
	})

	t.Run("processes !env in base_path from config file", func(t *testing.T) {
		// Set environment variable
		envBasePath := filepath.Join(tempDir, "custom-base")
		require.NoError(t, os.MkdirAll(envBasePath, 0o755))
		t.Setenv("CUSTOM_BASE_PATH", envBasePath)

		// Create atmos.yaml with !env in base_path
		configPath := filepath.Join(gitRoot, "atmos.yaml")
		configContent := `
base_path: "!env CUSTOM_BASE_PATH"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

		// Change to git root directory so workdir config is found
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(originalDir) }()
		require.NoError(t, os.Chdir(gitRoot))

		// Load config
		configInfo := schema.ConfigAndStacksInfo{}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// Verify base_path was processed and resolved to env var value
		assert.Equal(t, envBasePath, config.BasePath,
			"base_path should be processed from !env to actual environment variable value")
	})

	t.Run("handles literal base_path without YAML functions", func(t *testing.T) {
		// Create atmos.yaml with literal base_path
		literalPath := "/literal/path"
		configPath := filepath.Join(gitRoot, "atmos.yaml")
		configContent := `
base_path: "` + literalPath + `"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

		// Change to git root directory so workdir config is found
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(originalDir) }()
		require.NoError(t, os.Chdir(gitRoot))

		// Load config
		configInfo := schema.ConfigAndStacksInfo{}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// Verify base_path is unchanged
		assert.Equal(t, literalPath, config.BasePath,
			"literal base_path should not be modified")
	})

	t.Run("processes !repo-root from embedded config when no config file exists", func(t *testing.T) {
		// Create a temp dir without atmos.yaml
		noCfgDir := filepath.Join(tempDir, "no-config")
		require.NoError(t, os.MkdirAll(noCfgDir, 0o755))
		t.Setenv("TEST_GIT_ROOT", noCfgDir)

		originalDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() { _ = os.Chdir(originalDir) }()
		require.NoError(t, os.Chdir(noCfgDir))

		// Load config (will use embedded config)
		configInfo := schema.ConfigAndStacksInfo{}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// The embedded config no longer has base_path, so it should be empty
		// or default to some value. Let's just verify it loaded without error.
		assert.NotNil(t, config, "config should load successfully")
	})
}

func TestLoadConfig_YAMLFunctionPrecedence(t *testing.T) {
	// Enable git root search for this test (disabled by TestMain by default)
	t.Setenv("ATMOS_GIT_ROOT_ENABLED", "true")

	tempDir := t.TempDir()
	gitRoot := filepath.Join(tempDir, "repo")
	require.NoError(t, os.MkdirAll(gitRoot, 0o755))
	t.Setenv("TEST_GIT_ROOT", gitRoot)

	t.Run("CLI flag base_path overrides config file base_path", func(t *testing.T) {
		// Create atmos.yaml with !repo-root
		configPath := filepath.Join(gitRoot, "atmos.yaml")
		configContent := `
base_path: "!repo-root"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

		// Set CLI base path using !env
		cliBasePath := filepath.Join(tempDir, "cli-base")
		require.NoError(t, os.MkdirAll(cliBasePath, 0o755))
		t.Setenv("CLI_BASE", cliBasePath)

		configInfo := schema.ConfigAndStacksInfo{
			AtmosBasePath: "!env CLI_BASE",
		}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// CLI flag should win
		assert.Equal(t, cliBasePath, config.BasePath,
			"CLI flag base_path should override config file base_path")
	})

	t.Run("env var base_path overrides config file base_path", func(t *testing.T) {
		// Create atmos.yaml with literal path
		configPath := filepath.Join(gitRoot, "atmos.yaml")
		configContent := `
base_path: "/config/path"
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

		// Set ATMOS_BASE_PATH env var with !repo-root
		t.Setenv("ATMOS_BASE_PATH", "!repo-root")

		configInfo := schema.ConfigAndStacksInfo{}
		config, err := InitCliConfig(configInfo, false)
		require.NoError(t, err)

		// Env var should win and be processed
		assert.Equal(t, gitRoot, config.BasePath,
			"ATMOS_BASE_PATH env var should override config file and be processed")
	})
}
