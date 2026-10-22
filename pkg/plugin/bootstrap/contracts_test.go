package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConstructorSetRejectsDuplicateAndUnsupportedExports(t *testing.T) {
	t.Parallel()
	constructors := newConstructors(&adapter.Services{})
	provider := extension.Binding{Name: "example/provider", Capability: "auth.provider"}
	require.NoError(t, constructors.provider(provider, &adapter.Definition{}))
	require.ErrorIs(t, constructors.provider(provider, &adapter.Definition{}), errUtils.ErrDuplicateAuthRegistration)
	identity := extension.Binding{Name: "example/broker", Capability: "auth.identity", Metadata: map[string]any{"accepts": []any{"oidc/v1"}}}
	require.ErrorIs(t, constructors.identity(identity, &adapter.Definition{Outputs: []string{"oidc/v1"}}), errUtils.ErrPluginUnsupportedExtension)
	definition := &adapter.Definition{Outputs: []string{"aws/v1"}}
	require.NoError(t, constructors.identity(identity, definition))
	require.ErrorIs(t, constructors.identity(identity, definition), errUtils.ErrDuplicateAuthRegistration)
	identity.Name = "other"
	identity.Metadata["accepts"] = []any{42}
	require.ErrorIs(t, constructors.identity(identity, definition), errUtils.ErrPluginInvalidConfig)
	activation := &extension.Activation{Binding: extension.Binding{Name: "about"}, Package: "example/plugin"}
	require.NoError(t, constructors.command(t.Context(), activation))
	require.ErrorIs(t, constructors.command(t.Context(), activation), errUtils.ErrDuplicatePluginExtension)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, constructors.command(ctx, activation), context.Canceled)
	require.ErrorIs(t, constructors.activator("auth.provider")(ctx, activation), context.Canceled)
	assert.Nil(t, constructors.activator("unregistered"))
	require.ErrorIs(t, constructors.activator("auth.provider")(t.Context(), activation), errUtils.ErrPluginInvalidConfig)
}

func TestAuthDefinitionRequiresSchemaAndCredentialTypes(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schema.json"), []byte(`{"type":"object"}`), 0o600))
	constructors := newConstructors(&adapter.Services{})
	for _, metadata := range []map[string]any{
		{}, {"schema": "missing.json"}, {"schema": "schema.json"}, {"schema": "schema.json", "credentials": []any{}}, {"schema": "schema.json", "credentials": []any{42}}, {"schema": "schema.json", "credentials": []any{""}},
	} {
		_, err := constructors.authDefinition(&extension.Activation{Binding: extension.Binding{Directory: directory, Metadata: metadata}})
		require.Error(t, err)
	}
	activation := &extension.Activation{Binding: extension.Binding{Directory: directory, Metadata: map[string]any{"schema": "schema.json", "credentials": []any{"oidc/v1"}}}, Package: "example/oidc"}
	definition, err := constructors.authDefinition(activation)
	require.NoError(t, err)
	assert.Equal(t, []string{"oidc/v1"}, definition.Outputs)
	require.NoError(t, definition.Schema(map[string]any{}))
	require.Error(t, definition.Schema("invalid"))
}

func TestValidateExecutableRejectsUnsafeAndAbsentPaths(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("fixture"), 0o600))
	require.NoError(t, validateExecutable(directory, filepath.Join("bin", "plugin")))
	require.ErrorIs(t, validateExecutable(directory, "../outside"), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, validateExecutable(filepath.Join(directory, "absent"), "plugin"), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, validateExecutable(directory, "missing"), errUtils.ErrPluginNotInstalled)
	require.ErrorIs(t, validateExecutable(directory, "bin"), errUtils.ErrPluginInvalidConfig)
}

func TestCommandSelectionRequiresExactlyOneExport(t *testing.T) {
	t.Parallel()
	bindings := []extension.Binding{{Name: "one", Capability: "command"}, {Name: "two", Capability: "command"}, {Name: "auth", Capability: "auth.provider"}}
	for _, name := range []string{"", "missing", "auth"} {
		_, err := selectCommand(bindings, name)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	selected, err := selectCommand(bindings, "two")
	require.NoError(t, err)
	assert.Equal(t, bindings[1], selected)
	_, err = selectCommand(nil, "")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestProjectDirectoryAndEmptyPreparation(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ".", ProjectDirectory(&schema.AtmosConfiguration{}))
	assert.Equal(t, "base", ProjectDirectory(&schema.AtmosConfiguration{BasePath: "base"}))
	assert.Equal(t, "provenance", ProjectDirectory(&schema.AtmosConfiguration{BasePath: "base", BasePathConfigDir: "provenance"}))
	atmos := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	cleanup, err := Prepare(t.Context(), atmos)
	require.NoError(t, err)
	cleanup()
	require.ErrorIs(t, Run(t.Context(), atmos, "missing", nil, Output{}), errUtils.ErrPluginNotInstalled)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Prepare(ctx, atmos)
	require.ErrorIs(t, err, context.Canceled)
}

func TestActivationRejectsMissingPackageAndExecutable(t *testing.T) {
	t.Parallel()
	descriptors, err := Descriptors()
	require.NoError(t, err)
	entry := config.Selection{Path: t.TempDir(), Executable: "bin/plugin"}
	require.Error(t, activatePackage(t.Context(), &entry, descriptors))
	require.NoError(t, os.WriteFile(filepath.Join(entry.Path, "plugin.yaml"), []byte(automaticManifest), 0o600))
	require.ErrorIs(t, activatePackage(t.Context(), &entry, descriptors), errUtils.ErrPluginNotInstalled)
	require.NoError(t, os.Mkdir(filepath.Join(entry.Path, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(entry.Path, "bin", "plugin"), []byte("not executed"), 0o700))
	require.ErrorIs(t, activatePackage(t.Context(), &entry, descriptors), errUtils.ErrPluginUnsupportedExtension)

	// A path package names its alias and directory in the repair hint and keeps the sentinel.
	local := config.Selection{Path: t.TempDir(), Local: true}
	err = activatePackage(t.Context(), &local, descriptors)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "plugins.packages.fixture.path")
}

func activatePackage(ctx context.Context, entry *config.Selection, registry *extension.Registry) error {
	return activatePackageWithTimeout(ctx, "fixture", entry, registry, 0)
}
