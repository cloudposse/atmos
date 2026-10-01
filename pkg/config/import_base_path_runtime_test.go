package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveRuntimeBasePath verifies detection and precedence of the runtime base-path
// sources: the AtmosBasePath field (--base-path flag or atmos_base_path provider param)
// wins over the ATMOS_BASE_PATH environment variable, and empty values are unset.
func TestResolveRuntimeBasePath(t *testing.T) {
	t.Run("AtmosBasePath field wins over env var", func(t *testing.T) {
		t.Setenv("ATMOS_BASE_PATH", "/from/env")
		got := resolveRuntimeBasePath(&schema.ConfigAndStacksInfo{AtmosBasePath: "/from/flag"})
		assert.Equal(t, "/from/flag", got)
	})

	t.Run("env var used when field is empty", func(t *testing.T) {
		t.Setenv("ATMOS_BASE_PATH", "/from/env")
		got := resolveRuntimeBasePath(&schema.ConfigAndStacksInfo{})
		assert.Equal(t, "/from/env", got)
	})

	t.Run("nil info falls back to env var", func(t *testing.T) {
		t.Setenv("ATMOS_BASE_PATH", "/env/only")
		assert.Equal(t, "/env/only", resolveRuntimeBasePath(nil))
	})

	t.Run("empty when neither source is set", func(t *testing.T) {
		t.Setenv("ATMOS_BASE_PATH", "")
		assert.Empty(t, resolveRuntimeBasePath(&schema.ConfigAndStacksInfo{}))
	})

	t.Run("surrounding whitespace is trimmed", func(t *testing.T) {
		t.Setenv("ATMOS_BASE_PATH", "")
		got := resolveRuntimeBasePath(&schema.ConfigAndStacksInfo{AtmosBasePath: "  ./here  "})
		assert.Equal(t, "./here", got)
	})
}

// writeMaxWidthConfig writes a minimal config file that sets settings.terminal.max_width,
// used as an import target to prove where import resolution anchored.
func writeMaxWidthConfig(t *testing.T, path string, width int) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		path,
		fmt.Appendf(nil, "settings:\n  terminal:\n    max_width: %d\n", width),
		0o644,
	))
}

// TestMergeImports_RuntimeBasePathOverrideAnchorsToCWD verifies that a runtime dot
// base_path anchors imports to the current working directory, even when the config
// directory (where a config-sourced dot would anchor) is elsewhere.
func TestMergeImports_RuntimeBasePathOverrideAnchorsToCWD(t *testing.T) {
	setupTestAdapters()
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeMaxWidthConfig(t, filepath.Join(cwd, "extra.yaml"), 201)

	configDir := t.TempDir() // The import does not exist here.

	v := viper.New()
	v.SetConfigType(yamlType)
	v.Set("base_path", ".")
	v.Set("import", []string{"extra.yaml"})

	_, err := mergeImports(v, configDir, "", ".")
	require.NoError(t, err)
	assert.Equal(t, 201, v.GetInt("settings.terminal.max_width"),
		"runtime dot base_path must anchor imports to the CWD")
}

// TestMergeImports_ConfigDotBasePathAnchorsToConfigDir is the contrasting case: without a
// runtime override, a config-file dot base_path anchors imports to the config directory.
func TestMergeImports_ConfigDotBasePathAnchorsToConfigDir(t *testing.T) {
	setupTestAdapters()
	configDir := t.TempDir()
	writeMaxWidthConfig(t, filepath.Join(configDir, "extra.yaml"), 202)
	t.Chdir(t.TempDir()) // The import does not exist in the CWD.

	v := viper.New()
	v.SetConfigType(yamlType)
	v.Set("base_path", ".")
	v.Set("import", []string{"extra.yaml"})

	_, err := mergeImports(v, configDir, "", "")
	require.NoError(t, err)
	assert.Equal(t, 202, v.GetInt("settings.terminal.max_width"),
		"config dot base_path must anchor imports to the config dir")
}

// TestMergeConfigFileWithImports_RuntimeBasePathOverrideAnchorsToCWD verifies the profile
// merge path also honors a runtime override recorded on the main Viper instance.
func TestMergeConfigFileWithImports_RuntimeBasePathOverrideAnchorsToCWD(t *testing.T) {
	setupTestAdapters()
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeMaxWidthConfig(t, filepath.Join(cwd, "extra.yaml"), 203)

	configDir := t.TempDir()
	cfg := filepath.Join(configDir, "atmos.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("base_path: .\nimport:\n  - extra.yaml\n"), 0o644))

	v := viper.New()
	v.SetConfigType(yamlType)
	v.Set(runtimeBasePathOverrideKey, ".") // As LoadConfig records for ATMOS_BASE_PATH=. etc.

	require.NoError(t, mergeConfigFileWithImports(cfg, v))
	assert.Equal(t, 203, v.GetInt("settings.terminal.max_width"),
		"runtime dot base_path must anchor profile imports to the CWD")
}

// TestLoadConfig_RuntimeBasePathAnchorsImportsToCWD is the end-to-end case. A runtime dot
// base path set via the ATMOS_BASE_PATH env var flows through LoadConfig into import
// resolution so the imported file is found relative to the CWD, not the config directory.
func TestLoadConfig_RuntimeBasePathAnchorsImportsToCWD(t *testing.T) {
	setupTestAdapters()
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeMaxWidthConfig(t, filepath.Join(cwd, "extra.yaml"), 204)

	configDir := t.TempDir() // The import does not exist here.
	cfg := filepath.Join(configDir, "atmos.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("base_path: .\nimport:\n  - extra.yaml\n"), 0o644))

	t.Setenv("ATMOS_BASE_PATH", ".")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{AtmosConfigFilesFromArg: []string{cfg}})
	require.NoError(t, err)
	assert.Equal(t, 204, atmosConfig.Settings.Terminal.MaxWidth,
		"ATMOS_BASE_PATH=. must anchor imports to the CWD end to end")
}
