package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPackageTimeoutReachesTheActivation(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("static"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	constructors := newConstructors(&adapter.Services{})
	registry, err := descriptors(constructors)
	require.NoError(t, err)
	state := &config.Selections{Packages: map[string]config.Selection{"fixture": {Path: directory, Executable: "bin/plugin"}}}

	require.NoError(t, activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {Timeout: "5m"}}))
	require.Len(t, constructors.Commands, 1)
	for _, activation := range constructors.Commands {
		assert.Equal(t, 5*time.Minute, activation.Timeout)
	}

	// Negative: no timeout configured means no deadline.
	constructors = newConstructors(&adapter.Services{})
	registry, err = descriptors(constructors)
	require.NoError(t, err)
	require.NoError(t, activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {}}))
	for _, activation := range constructors.Commands {
		assert.Zero(t, activation.Timeout)
	}

	// An invalid duration fails with a configuration error rather than running unbounded.
	constructors = newConstructors(&adapter.Services{})
	registry, err = descriptors(constructors)
	require.NoError(t, err)
	err = activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": {Timeout: "soon"}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestPackageInteractionTimeoutReachesTheActivation(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("static"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(automaticManifest, "PLATFORM", install.CurrentPlatform())), 0o600))
	state := &config.Selections{Packages: map[string]config.Selection{"fixture": {Path: directory, Executable: "bin/plugin"}}}
	activate := func(t *testing.T, declaration schema.PluginPackage) (*ConstructorSet, error) {
		t.Helper()
		constructors := newConstructors(&adapter.Services{})
		registry, err := descriptors(constructors)
		require.NoError(t, err)
		return constructors, activatePackages(t.Context(), state, registry, map[string]schema.PluginPackage{"fixture": declaration})
	}

	constructors, err := activate(t, schema.PluginPackage{Timeout: "5m", InteractionTimeout: "45m"})
	require.NoError(t, err)
	require.Len(t, constructors.Commands, 1)
	for _, activation := range constructors.Commands {
		assert.Equal(t, 5*time.Minute, activation.Timeout)
		assert.Equal(t, 45*time.Minute, activation.InteractionLimit)
		assert.Equal(t, "fixture", activation.Alias, "the alias lets errors name the package's configuration keys")
	}

	// Negative: no interaction_timeout leaves the zero value, which selects the runtime default.
	constructors, err = activate(t, schema.PluginPackage{Timeout: "5m"})
	require.NoError(t, err)
	require.Len(t, constructors.Commands, 1)
	for _, activation := range constructors.Commands {
		assert.Zero(t, activation.InteractionLimit)
	}

	// An invalid duration fails with a configuration error instead of falling back to the default.
	_, err = activate(t, schema.PluginPackage{Timeout: "5m", InteractionTimeout: "soon"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestAuthDefinitionCarriesTheInteractionLimit(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schema.json"), []byte(`{"type":"object"}`), 0o600))
	constructors := newConstructors(&adapter.Services{})
	activation := &extension.Activation{
		Binding: extension.Binding{Directory: directory, Metadata: map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v1"}}},
		Package: "example/oidc", Alias: "corporate", Timeout: 5 * time.Minute, InteractionLimit: 45 * time.Minute,
	}
	definition, err := constructors.authDefinition(activation)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, definition.Timeout)
	assert.Equal(t, 45*time.Minute, definition.InteractionLimit)
	assert.Equal(t, "corporate", definition.Alias)
}
