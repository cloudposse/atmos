package configuration

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFactoryRegistrationAndOptIn(t *testing.T) {
	factories.Lock()
	previous := factories.values
	factories.values = make(map[string]Factory)
	factories.Unlock()
	t.Cleanup(func() { factories.Lock(); factories.values = previous; factories.Unlock() })
	config := &schema.AtmosConfiguration{}
	require.NoError(t, Attach(t.Context(), config, "list"))
	require.Nil(t, config.ComponentConfigurations)
	settings := schema.ComponentConfiguration{Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"sample": settings}
	require.ErrorIs(t, Attach(t.Context(), config, "list"), errUtils.ErrPluginUnsupportedExtension)
	require.ErrorIs(t, Register("", func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		return nil, nil
	}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, Register("plugin", nil), errUtils.ErrPluginInvalidConfig)
	called := false
	factory := func(ctx context.Context, got *schema.AtmosConfiguration, operation string) (schema.ComponentConfigurationRuntime, error) {
		called = true
		require.Same(t, config, got)
		require.Equal(t, "list", operation)
		return nil, errUtils.ErrPluginInvalidConfig
	}
	require.NoError(t, Register("plugin", factory))
	require.ErrorIs(t, Register("plugin", factory), errUtils.ErrDuplicatePluginExtension)
	require.False(t, called)
	require.ErrorIs(t, Attach(t.Context(), config, "list"), errUtils.ErrPluginInvalidConfig)
	require.True(t, called)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, Attach(ctx, config, "list"), context.Canceled)
	factories.Lock()
	factories.values["plugin"] = func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		return nil, nil
	}
	factories.Unlock()
	require.NoError(t, Attach(t.Context(), config, "list"))
}

type importRuntime struct {
	schema.ComponentConfigurationRuntime
	imports []string
	err     error
}

func (r *importRuntime) ImportComponents(_ context.Context, uri string) (map[string]any, bool, error) {
	r.imports = append(r.imports, uri)
	return map[string]any{"app": map[string]any{}}, true, r.err
}

func TestConfigurationImportRouting(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	for _, uri := range []string{"ordinary", "https://example.com/config.yaml"} {
		_, handled, err := ImportComponents(config, uri, &schema.StackImport{}, false)
		require.NoError(t, err)
		require.False(t, handled)
	}
	_, handled, err := ImportComponents(nil, "terragrunt://live", &schema.StackImport{}, false)
	require.NoError(t, err)
	require.False(t, handled)
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {}}
	_, _, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.ErrorContains(t, err, "disabled")
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = schema.ComponentConfiguration{Enabled: true}
	_, _, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	runtime := &importRuntime{}
	config.ComponentConfigurations = runtime
	for _, declaration := range []schema.StackImport{{Context: map[string]any{"stage": "dev"}}, {SkipIfMissing: true}, {TTL: "1h"}, {NestedImports: "local"}} {
		_, _, err := ImportComponents(config, "terragrunt://live", &declaration, false)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	}
	_, handled, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []string{"terragrunt://live"}, runtime.imports)
	runtime.err = errors.New("registration failed")
	_, _, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.ErrorIs(t, err, runtime.err)
}

func TestWatchExcludes(t *testing.T) {
	patterns, err := WatchExcludes(map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/.cache/**"}}})
	require.NoError(t, err)
	root := t.TempDir()
	files := []string{filepath.Join(root, "a", ".cache", "x"), filepath.Join(root, "a", "main.tf")}
	filtered, err := UnexcludedWatchFiles(root, files, patterns)
	require.NoError(t, err)
	require.Equal(t, files[1:], filtered)
	for _, value := range []any{"bad shape", []any{"["}, []any{42}} {
		_, err := WatchExcludes(map[string]any{"configuration": map[string]any{"watch_excludes": value}})
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	_, err = UnexcludedWatchFiles("relative", files, patterns)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}
