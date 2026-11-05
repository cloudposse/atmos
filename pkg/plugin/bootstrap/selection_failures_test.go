package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

func developmentCommandFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	directory, project := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("not executed"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	atmos := &schema.AtmosConfiguration{BasePathConfigDir: project}
	atmos.Plugins.Packages = map[string]schema.PluginPackage{"fixture": {Path: directory}}
	return atmos
}

func TestCommandSelectionReportsInvalidLocalBuilds(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"missing alias", "missing manifest", "unsupported extension", "missing executable", "executable directory", "missing command"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			atmos := developmentCommandFixture(t)
			directory := atmos.Plugins.Packages["fixture"].Path
			alias := "fixture"
			expected := errUtils.ErrPluginInvalidConfig
			switch failure {
			case "missing alias":
				alias, expected = "absent", errUtils.ErrPluginNotInstalled
			case "missing manifest":
				require.NoError(t, os.Remove(filepath.Join(directory, "plugin.yaml")))
			case "unsupported extension":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform()), "commands:", "unsupported:")), 0o600))
				expected = errUtils.ErrPluginUnsupportedExtension
			case "missing executable", "executable directory":
				require.NoError(t, os.Remove(filepath.Join(directory, "bin", "plugin")))
				if failure == "executable directory" {
					require.NoError(t, os.Mkdir(filepath.Join(directory, "bin", "plugin"), 0o700))
				} else {
					expected = errUtils.ErrPluginNotInstalled
				}
			case "missing command":
				declaration := atmos.Plugins.Packages["fixture"]
				declaration.Command = "absent"
				atmos.Plugins.Packages["fixture"] = declaration
			}
			require.ErrorIs(t, Run(t.Context(), atmos, alias, nil, Output{}), expected)
		})
	}
}

func TestActivationRejectsUnavailableManifestAndMissingActivator(t *testing.T) {
	t.Parallel()
	atmos := developmentCommandFixture(t)
	entry := config.Selection{Path: atmos.Plugins.Packages["fixture"].Path, Local: true}
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{Capability: "command", Path: []string{"commands"}, Version: 1, Validate: extension.ValidateCommand}))
	require.ErrorIs(t, activatePackageWithTimeout(t.Context(), "fixture", &entry, registry, 0), errUtils.ErrPluginUnsupportedExtension)
	require.NoError(t, os.Remove(filepath.Join(entry.Path, "plugin.yaml")))
	require.ErrorIs(t, activatePackageWithTimeout(t.Context(), "fixture", &entry, registry, 0), errUtils.ErrPluginInvalidConfig)
}

func TestCommandConfigurationValidatedBeforeProcessStartup(t *testing.T) {
	t.Parallel()
	atmos := developmentCommandFixture(t)
	directory := atmos.Plugins.Packages["fixture"].Path
	definition := strings.Replace(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform()), "about: {description: Static automatic-install fixture}", "about: {description: Static automatic-install fixture, schema: settings.json}", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(definition), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"type":"object","properties":{"tenant":{"type":"string"}},"required":["tenant"]}`), 0o600))
	require.ErrorIs(t, Run(t.Context(), atmos, "fixture", nil, Output{}), errUtils.ErrPluginInvalidConfig)
	atmos.Plugins.Packages = map[string]schema.PluginPackage{"fixture": {Path: directory, Config: map[string]any{"tenant": 42}}}
	require.ErrorIs(t, Run(t.Context(), atmos, "fixture", nil, Output{}), errUtils.ErrPluginInvalidConfig)
	atmos.Plugins.Packages["fixture"] = schema.PluginPackage{Path: directory, Config: map[string]any{"tenant": "valid"}}
	require.ErrorIs(t, Run(t.Context(), atmos, "fixture", nil, Output{}), errUtils.ErrPluginProtocol)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	atmos.Plugins.Packages["fixture"] = schema.PluginPackage{Path: directory, Config: map[string]any{"unsupported": make(chan int)}}
	require.ErrorIs(t, Run(t.Context(), atmos, "fixture", nil, Output{}), errUtils.ErrPluginInvalidConfig)
}

func TestPathSelectionRejectsInvalidConfigurationAndPendingTransactions(t *testing.T) {
	t.Parallel()
	atmos := developmentCommandFixture(t)
	state, err := selectedPackages(t.Context(), atmos)
	require.NoError(t, err)
	assert.True(t, state.Packages["fixture"].Local)
	declaration := atmos.Plugins.Packages["fixture"]
	declaration.Path = "   "
	atmos.Plugins.Packages["fixture"] = declaration
	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, os.MkdirAll(filepath.Join(atmos.BasePathConfigDir, ".atmos", "plugins"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(atmos.BasePathConfigDir, ".atmos", "plugins", "transaction.json"), []byte("[]"), 0o600))
	declaration.Path = t.TempDir()
	atmos.Plugins.Packages["fixture"] = declaration
	_, err = selectedPackages(t.Context(), atmos)
	require.ErrorIs(t, err, errUtils.ErrPluginTransactionPending)
}

func TestLockedSelectionRejectsMissingDriftedAndUncachedEntries(t *testing.T) {
	t.Parallel()
	file := &pluginlock.File{}
	declaration := &schema.PluginPackage{Source: "oci://example.com/plugin", Version: "1.0.0"}
	cache, err := install.NewCache(t.TempDir())
	require.NoError(t, err)
	_, err = lockedSelection("fixture", declaration, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	file.Spec.Packages = map[string]pluginlock.Entry{"fixture": {Source: declaration.Source, Constraint: "2.0.0"}}
	_, err = lockedSelection("fixture", declaration, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginFrozenDrift)
	entry := file.Spec.Packages["fixture"]
	entry.Constraint = declaration.Version
	file.Spec.Packages["fixture"] = entry
	_, err = lockedSelection("fixture", declaration, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	state := config.NewSelections()
	atmos := &schema.AtmosConfiguration{}
	atmos.Plugins.Packages = map[string]schema.PluginPackage{"fixture": *declaration}
	_, err = applySelections(atmos, state, file, cache)
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
}

func TestActivationStopsWhenMetadataDisappearsOrRegistrationFails(t *testing.T) {
	t.Parallel()
	for _, disappear := range []bool{false, true} {
		t.Run(fmt.Sprint(disappear), func(t *testing.T) {
			t.Parallel()
			atmos := developmentCommandFixture(t)
			directory := atmos.Plugins.Packages["fixture"].Path
			entry := config.Selection{Path: directory, Local: true}
			registry := extension.New()
			failure := errors.New("extension registration failed")
			activated := false
			require.NoError(t, registry.Register(extension.Descriptor{
				Capability: "command", Path: []string{"commands"}, Version: 1,
				Validate: func(extension.Binding) error {
					if disappear {
						return os.Remove(filepath.Join(directory, "plugin.yaml"))
					}
					return nil
				},
				Activate: func(context.Context, *extension.Activation) error { activated = true; return failure },
			}))
			err := activatePackageWithTimeout(t.Context(), "fixture", &entry, registry, 0)
			if disappear {
				require.ErrorIs(t, err, os.ErrNotExist)
				require.False(t, activated, "metadata must still be readable when deriving the required handshake")
			} else {
				require.ErrorIs(t, err, failure)
				require.True(t, activated)
			}
		})
	}
}
