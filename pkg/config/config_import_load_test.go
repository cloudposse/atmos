package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestMergeConfig_RequiredImportFailsClosed is an integration-level guard that a required
// (object-form) import whose target is missing aborts the full config-load path
// (mergeConfig -> processConfigImportsAndReapply -> mergeImports), rather than being swallowed as
// a debug log. This is the end-to-end counterpart to the unit tests in config_import_dx_test.go.
func TestMergeConfig_RequiredImportFailsClosed(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)

	tempDir := t.TempDir()
	createConfigFile(t, tempDir, "atmos.yaml", `
base_path: ./
import:
  - path: "configs.d/does-not-exist.yaml"
    skip_if_missing: false
`)

	v := viper.New()
	v.SetConfigType("yaml")
	err := mergeConfig(v, tempDir, CliConfigFileName, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredImportFailed)
}

// TestMergeConfig_OptionalImportFailsOpen is the backward-compatibility counterpart: a legacy
// string (optional) import whose target is missing is warned and skipped, and config loading
// succeeds.
func TestMergeConfig_OptionalImportFailsOpen(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)

	tempDir := t.TempDir()
	createConfigFile(t, tempDir, "atmos.yaml", `
base_path: ./
import:
  - "configs.d/does-not-exist.yaml"
logs:
  level: Info
`)

	v := viper.New()
	v.SetConfigType("yaml")
	err := mergeConfig(v, tempDir, CliConfigFileName, true)
	require.NoError(t, err, "a missing string (optional) import must not abort the config load")
	assert.Equal(t, "Info", v.GetString("logs.level"), "the rest of the config still loads")
}
