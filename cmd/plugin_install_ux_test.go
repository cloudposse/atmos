package cmd

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

func formatPluginError(err error) string {
	return errUtils.Format(err, errUtils.DefaultFormatterConfig())
}

func TestPluginInstallPathSourcesAreRejectedBeforeAnyRegistryLookup(t *testing.T) {
	config := pluginCoverageProject(t)
	// A configured catalog on an unroutable host would hang or fail differently if it were queried.
	config.Plugins.Registries = map[string]schema.PluginRegistry{"local": {Catalog: "oci://127.0.0.1:1/catalog:latest"}}
	for _, source := range []string{filepath.Join(config.BasePath, "plugin"), "./plugin", "../plugin", "~/plugin"} {
		_, err := invokePluginCoverage(t, config, "install", source)
		require.ErrorIs(t, err, errUtils.ErrPluginSourceIsPath, source)
		formatted := formatPluginError(err)
		assert.Contains(t, formatted, "declare path:")
		assert.Contains(t, formatted, "plugins.packages.<alias>")
		assert.NotContains(t, formatted, "--dev")
	}
	for _, source := range []string{"local/", "/", "local//x"} {
		_, err := invokePluginCoverage(t, config, "install", source)
		require.Error(t, err, source)
		assert.Condition(t, func() bool {
			return errorsIsAny(err, errUtils.ErrPluginInvalidConfig, errUtils.ErrPluginSourceIsPath)
		}, source)
	}
	assert.NoFileExists(t, pluginlock.Path(config.BasePath), "a rejected source must not write the lock")
}

func errorsIsAny(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func TestPluginInstallWithoutSourceRejectsIgnoredFlags(t *testing.T) {
	config := pluginCoverageProject(t)
	for _, flag := range []string{"--as=x", "--version=^1"} {
		_, err := invokePluginCoverage(t, config, "install", flag)
		require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict, flag)
		assert.Contains(t, formatPluginError(err), "source")
	}
}

func TestPluginUpdateVersionRequiresAnAlias(t *testing.T) {
	config := pluginCoverageProject(t)
	_, err := invokePluginCoverage(t, config, "update", "--version=^1")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled, "a missing alias is a usage error, not a missing package")
	assert.Contains(t, formatPluginError(err), "atmos plugin update <alias> --version")
	_, err = invokePluginCoverage(t, config, "update", "missing", "--version=^1")
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	assert.Contains(t, formatPluginError(err), "atmos plugin list")
}
