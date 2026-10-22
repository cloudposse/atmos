package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

func TestCreateRejectsUnsupportedOrIncompleteOptions(t *testing.T) {
	t.Parallel()
	baseline := Options{Name: "example/oidc", Module: "example.com/oidc", Runtime: "go", Capability: "auth.provider", SDKVersion: "v1.233.0", Defaults: true}
	for _, test := range []struct {
		name   string
		change func(*Options)
	}{
		{"runtime", func(o *Options) { o.Runtime = "starlark" }},
		{"capability", func(o *Options) { o.Capability = "store" }},
		{"module", func(o *Options) { o.Module = "invalid module" }},
		{"sdk", func(o *Options) { o.SDKVersion = "not-a-version" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			options := baseline
			test.change(&options)
			directory := filepath.Join(t.TempDir(), "plugin")
			require.ErrorIs(t, Create(t.Context(), directory, options), errUtils.ErrPluginInvalidConfig)
			_, err := os.Stat(directory)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, Create(ctx, filepath.Join(t.TempDir(), "plugin"), baseline), context.Canceled)
}

func TestScaffoldManifestUsesRunnableExecutableForTargetPlatform(t *testing.T) {
	configurations, err := templates.GetAvailableConfigurations()
	require.NoError(t, err)
	configuration, ok := configurations["plugin-oidc"]
	require.True(t, ok)
	var source templates.File
	for _, file := range configuration.Files {
		if file.Path == "plugin.yaml.tmpl" {
			source = file
		}
	}
	require.NotEmpty(t, source.Content)
	for _, tc := range []struct{ platform, executable string }{
		{"windows/amd64", "bin/plugin.exe"},
		{"windows/arm64", "bin/plugin.exe"},
		{"linux/amd64", "bin/plugin"},
		{"darwin/arm64", "bin/plugin"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			directory := t.TempDir()
			values := map[string]any{"plugin_name": "example/oidc", "display_name": "OIDC", "description": "OIDC login", "platform": tc.platform}
			require.NoError(t, engine.NewProcessor().ProcessFile(engine.File{Path: "plugin.yaml", Content: source.Content, IsTemplate: true, Permissions: source.Permissions}, directory, false, false, nil, values))
			definition, err := manifest.Load(directory)
			require.NoError(t, err)
			require.Equal(t, tc.executable, definition.Spec.Runtime.Executable)
			require.Equal(t, tc.executable, manifest.Executable(definition, tc.platform))
		})
	}
}

func TestCreateReleasedSDKAndProtectExistingSource(t *testing.T) {
	// The generator owns shared terminal state; do not run generation concurrently.
	directory := filepath.Join(t.TempDir(), "plugin")
	options := Options{Name: "example/oidc", Module: "example.com/oidc", Runtime: "go", Capability: "auth.provider", SDKVersion: "v1.233.0", Defaults: true, DisplayName: "Corporate Login", Description: "Sign in to corporate services", Author: "Example", Repository: "https://github.com/example/oidc"}
	require.NoError(t, Create(t.Context(), directory, options))
	module, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	require.NoError(t, err)
	require.Contains(t, string(module), "module example.com/oidc")
	require.Contains(t, string(module), "github.com/cloudposse/atmos/sdk v1.233.0")
	readme, err := os.ReadFile(filepath.Join(directory, "README.md"))
	require.NoError(t, err)
	require.Contains(t, string(readme), "atmos plugin build")
	manifest, err := os.ReadFile(filepath.Join(directory, "plugin.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifest), "https://github.com/example/oidc")
	require.Contains(t, string(manifest), "Corporate Login")
	require.Contains(t, string(manifest), "Sign in to corporate services")
	require.Contains(t, string(manifest), "Example")
	_, err = os.Stat(filepath.Join(directory, "go.work"))
	require.ErrorIs(t, err, os.ErrNotExist)
	source := filepath.Join(directory, "main.go")
	require.NoError(t, os.WriteFile(source, []byte("// local author edit\n"), 0o600))
	require.Error(t, Create(t.Context(), directory, options))
	preserved, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, "// local author edit\n", string(preserved))
}
