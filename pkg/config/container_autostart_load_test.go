package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const envContainerRuntimeAutoStart = "ATMOS_CONTAINER_RUNTIME_AUTO_START"

// TestLoadConfig_BridgesContainerAutoStartToEnv verifies the first-class YAML
// setting `container.runtime.auto_start: true` is promoted into the
// ATMOS_CONTAINER_RUNTIME_AUTO_START env var that container runtime detection reads.
func TestLoadConfig_BridgesContainerAutoStartToEnv(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\ncontainer:\n  runtime:\n    provider: podman\n    auto_start: true\n"), 0o644))

	t.Chdir(tmpDir)
	// Ensure the env var is unset so we observe config-driven promotion.
	t.Setenv(envContainerRuntimeAutoStart, "")
	require.NoError(t, os.Unsetenv(envContainerRuntimeAutoStart))

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.True(t, atmosConfig.Container.Runtime.AutoStart)
	assert.Equal(t, "podman", atmosConfig.Container.Runtime.Provider)
	assert.Equal(t, "true", os.Getenv(envContainerRuntimeAutoStart), "config should promote the auto-start env var")
}

// TestLoadConfig_ContainerAutoStartEnvWins verifies env precedence: an explicit
// ATMOS_CONTAINER_RUNTIME_AUTO_START is never overwritten by config.
func TestLoadConfig_ContainerAutoStartEnvWins(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\ncontainer:\n  runtime:\n    auto_start: true\n"), 0o644))

	t.Chdir(tmpDir)
	t.Setenv(envContainerRuntimeAutoStart, "false") // explicit env wins over config.

	_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, "false", os.Getenv(envContainerRuntimeAutoStart), "explicit env var must not be overwritten by config")
}

// TestLoadConfig_ContainerAutoStartDefaultsToTrue verifies that Podman recovery
// is on by default for every config, including ones that omit the namespace.
func TestLoadConfig_ContainerAutoStartDefaultsToTrue(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\n"), 0o644))

	t.Chdir(tmpDir)
	t.Setenv(envContainerRuntimeAutoStart, "")
	require.NoError(t, os.Unsetenv(envContainerRuntimeAutoStart))

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.True(t, atmosConfig.Container.Runtime.AutoStart)
	assert.Equal(t, "true", os.Getenv(envContainerRuntimeAutoStart))
}

// TestLoadConfig_ContainerAutoStartCanBeDisabled verifies an explicit config
// opt-out does not promote the recovery flag.
func TestLoadConfig_ContainerAutoStartCanBeDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\ncontainer:\n  runtime:\n    auto_start: false\n"), 0o644))

	t.Chdir(tmpDir)
	t.Setenv(envContainerRuntimeAutoStart, "")
	require.NoError(t, os.Unsetenv(envContainerRuntimeAutoStart))

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.False(t, atmosConfig.Container.Runtime.AutoStart)
	_, set := os.LookupEnv(envContainerRuntimeAutoStart)
	assert.False(t, set)
}
