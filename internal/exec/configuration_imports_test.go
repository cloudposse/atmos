package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

type importRuntime struct {
	*MockComponentConfigurationRuntime
	imports []string
	err     error
}

func (r *importRuntime) RegisterImport(_ context.Context, uri string) (bool, error) {
	r.imports = append(r.imports, uri)
	return true, r.err
}

func TestConfigurationImportRouting(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	for _, uri := range []string{"ordinary", "https://example.com/config.yaml"} {
		handled, err := registerConfigurationImport(config, uri, &schema.StackImport{})
		require.NoError(t, err)
		require.False(t, handled)
	}
	handled, err := registerConfigurationImport(nil, "terragrunt://live", &schema.StackImport{})
	require.NoError(t, err)
	require.False(t, handled)
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {}}
	_, err = registerConfigurationImport(config, "terragrunt://live", &schema.StackImport{})
	require.ErrorContains(t, err, "disabled")
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = schema.ComponentConfiguration{Enabled: true}
	_, err = registerConfigurationImport(config, "terragrunt://live", &schema.StackImport{})
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t))}
	config.ComponentConfigurations = runtime
	for _, declaration := range []schema.StackImport{{Context: map[string]any{"stage": "dev"}}, {SkipIfMissing: true}, {TTL: "1h"}, {NestedImports: "local"}} {
		_, err := registerConfigurationImport(config, "terragrunt://live", &declaration)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	}
	handled, err = registerConfigurationImport(config, "terragrunt://live", &schema.StackImport{})
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, []string{"terragrunt://live"}, runtime.imports)
	runtime.err = errors.New("registration failed")
	_, err = registerConfigurationImport(config, "terragrunt://live", &schema.StackImport{})
	require.ErrorIs(t, err, runtime.err)
}

func TestYAMLImportsRegisterBeforeDiscovery(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "adopt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("import:\n  - terragrunt://live\nvars:\n  stage: dev\n"), 0o600))
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t))}
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	_, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, []string{"terragrunt://live"}, runtime.imports)
}

func TestConfigurationWatchExclusionsDoNotChangeSharedIndex(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root}
	index := newChangedFilesIndex(config, []string{filepath.Join("live", ".terragrunt-cache", "generated.tf"), filepath.Join("live", "new.yaml")}, root)
	settings := map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/.terragrunt-cache/**"}}}
	filtered, err := configurationWatchIndex(config, settings, index)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(root, "live", "new.yaml")}, filtered.getAllFiles())
	require.Len(t, index.getAllFiles(), 2)
	unchanged, err := configurationWatchIndex(config, nil, index)
	require.NoError(t, err)
	require.Same(t, index, unchanged)
	for _, invalid := range []any{"[", 42} {
		settings["configuration"].(map[string]any)["watch_excludes"] = []any{invalid}
		_, err = configurationWatchIndex(config, settings, index)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
}

func TestConfigurationWatchExclusionsFailClosedOnPathsOutsideTheProject(t *testing.T) {
	// A relative project root cannot be compared with absolute changed files; the watch must not silently match nothing.
	config := &schema.AtmosConfiguration{BasePathAbsolute: "relative-root"}
	absolute := filepath.Join(t.TempDir(), "live", "new.yaml")
	index := newChangedFilesIndex(config, []string{absolute}, "")
	settings := map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/*.tf"}}}
	filtered, err := configurationWatchIndex(config, settings, index)
	require.Error(t, err)
	require.Nil(t, filtered)
}

func TestDependencyChangeCheckReportsInvalidWatchExclusions(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: root}
	index := newChangedFilesIndex(config, []string{filepath.Join("live", "changed.hcl")}, root)
	component := map[string]any{}
	settings := map[string]any{"configuration": map[string]any{"watch_excludes": []any{"["}}}
	var affected []schema.Affected
	err := checkDependencyChangesIndexed(&affected, config, "app", "dev", "terraform", &component, settings, index, false, nil, false)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Empty(t, affected, "an invalid exclusion must not be treated as a clean comparison")
}

func TestYAMLImportsRejectProviderImportsThatCannotBeRegistered(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "adopt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("import:\n  - terragrunt://live\nvars:\n  stage: dev\n"), 0o600))
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), err: errUtils.ErrPluginInvalidConfig}
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	_, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, "a rejected provider import must fail the stack load instead of being skipped")
	require.Equal(t, []string{"terragrunt://live"}, runtime.imports)

	// A disabled provider fails the same way before any plugin is consulted.
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: false}}
	_, err = ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Len(t, runtime.imports, 1, "the disabled provider is never asked to register the import")
}
