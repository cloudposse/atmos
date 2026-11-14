package configuration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_import_runtime_test.go -package=$GOPACKAGE -mock_names=importingRuntime=MockImportingRuntime

// importingRuntime is a configuration runtime that also resolves provider imports.
type importingRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationImportRuntime
}

// The generated mock implements the contract that ImportComponents asserts.
var _ importingRuntime = (*MockImportingRuntime)(nil)

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
	// Every expectation is explicit: an unexpected ImportComponents call, such as one from a
	// static load or a rejected import declaration, fails the test.
	runtime := NewMockImportingRuntime(gomock.NewController(t))
	config.ComponentConfigurations = runtime
	for _, declaration := range []schema.StackImport{{Context: map[string]any{"stage": "dev"}}, {SkipIfMissing: true}, {TTL: "1h"}, {NestedImports: "local"}} {
		_, _, err := ImportComponents(config, "terragrunt://live", &declaration, false)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	}
	_, handled, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, true)
	require.True(t, handled)
	require.ErrorIs(t, err, errUtils.ErrInvalidImport, "static loads must not enter the provider runtime")
	registration := errors.New("registration failed")
	gomock.InOrder(
		runtime.EXPECT().ImportComponents(gomock.Any(), "terragrunt://live").Return(map[string]any{"app": map[string]any{}}, true, nil),
		runtime.EXPECT().ImportComponents(gomock.Any(), "terragrunt://live").Return(map[string]any{"app": map[string]any{}}, true, registration),
	)
	fragment, handled, err := ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, map[string]any{"components": map[string]any{"terraform": map[string]any{"app": map[string]any{}}}}, fragment)
	_, _, err = ImportComponents(config, "terragrunt://live", &schema.StackImport{}, false)
	require.ErrorIs(t, err, registration)
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

func TestApplyUnitOverridesCopiesEachUnit(t *testing.T) {
	unit := map[string]any{"vars": map[string]any{"a": 1}}
	terraform := map[string]any{"app": unit, "invalid": "not a section"}
	fragments := []map[string]any{{"components": map[string]any{"terraform": terraform}}, {}}

	ApplyUnitOverrides(fragments, nil)
	require.Equal(t, map[string]any{"vars": map[string]any{"a": 1}}, terraform["app"], "empty overrides must leave units untouched")

	overrides := map[string]any{"vars": map[string]any{"region": "us-west-2"}}
	ApplyUnitOverrides(fragments, overrides)
	app := terraform["app"].(map[string]any)
	require.Equal(t, overrides, app["overrides"])
	require.Equal(t, map[string]any{"a": 1}, app["vars"])
	require.NotContains(t, unit, "overrides", "the provider's unit must not be changed")
	require.Equal(t, "not a section", terraform["invalid"])

	unit["vars"] = "changed later"
	require.Equal(t, map[string]any{"a": 1}, app["vars"], "replacing a source section must not reach the attached unit")
}

func TestMatchDependencies(t *testing.T) {
	deps := []schema.ComponentDependency{{Kind: "folder", Path: "short/unit"}, {Kind: "file", Path: "short/a.tf"}}
	resolve := func(path string) string { return "long/" + path }

	t.Run("YAML dependencies keep exact matching", func(t *testing.T) {
		require.Equal(t, deps, MatchDependencies(deps, false, resolve))
	})
	t.Run("provider watches are canonicalized without mutating input", func(t *testing.T) {
		matched := MatchDependencies(deps, true, resolve)
		require.Equal(t, "long/short/unit", matched[0].Path)
		require.Equal(t, "long/short/a.tf", matched[1].Path)
		require.Equal(t, "file", matched[1].Kind)
		require.Equal(t, "short/unit", deps[0].Path, "configured spelling must be preserved")
		require.Equal(t, "short/unit", ConfiguredDependencyPath(deps, matched, "long/short/unit"))
		require.Equal(t, "short/a.tf", ConfiguredDependencyPath(deps, matched, "long/short/a.tf"))
		require.Equal(t, "other", ConfiguredDependencyPath(deps, matched, "other"))
	})
	t.Run("nil resolver leaves dependencies unchanged", func(t *testing.T) {
		require.Equal(t, deps, MatchDependencies(deps, true, nil))
	})
}

func writeManifestSchema(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const testManifestSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"name":{"type":"string"}}}`

func TestValidateDiscoveredManifests(t *testing.T) {
	path := writeManifestSchema(t, testManifestSchema)
	compiles := 0
	compile := func() (*jsonschema.Schema, error) {
		compiles++
		return CompileManifestSchema(path)
	}

	messages, err := ValidateDiscoveredManifests(nil, compile)
	require.NoError(t, err)
	require.Empty(t, messages)
	require.Zero(t, compiles, "no discovered stacks must not compile the schema")

	discovered := map[string]map[string]any{
		"zeta":  {"name": 7},
		"alpha": {"name": false},
		"ok":    {"name": "ok"},
	}
	messages, err = ValidateDiscoveredManifests(discovered, compile)
	require.NoError(t, err)
	require.Equal(t, 1, compiles)
	require.Len(t, messages, 2)
	require.Contains(t, messages[0], "discovered stack 'alpha'", "messages are ordered by stack name")
	require.Contains(t, messages[1], "discovered stack 'zeta'")

	_, err = ValidateDiscoveredManifests(discovered, func() (*jsonschema.Schema, error) { return nil, errUtils.ErrOpenFile })
	require.ErrorIs(t, err, errUtils.ErrOpenFile)
}

func TestCompileManifestSchemaWrapsFailures(t *testing.T) {
	_, err := CompileManifestSchema(filepath.Join(t.TempDir(), "missing.json"))
	require.ErrorIs(t, err, errUtils.ErrOpenFile)
	require.ErrorIs(t, err, os.ErrNotExist, "the underlying chain must remain inspectable")

	_, err = CompileManifestSchema(writeManifestSchema(t, "{not json"))
	require.ErrorIs(t, err, errUtils.ErrStackManifestSchemaValidation)

	_, err = CompileManifestSchema(writeManifestSchema(t, `{"type": 12}`))
	require.ErrorIs(t, err, errUtils.ErrStackManifestSchemaValidation)

	compiled, err := CompileManifestSchema(writeManifestSchema(t, testManifestSchema))
	require.NoError(t, err)
	require.NotNil(t, compiled)
}

// Only a provider owns the backend; an empty `settings.configuration` map leaves ordinary components
// alone. Each component-level layer, inherited or the component's own, is checked.
func TestBackendOverridesRequireProviderOwnership(t *testing.T) {
	s3 := BackendLayer{Type: "s3", Settings: map[string]any{"bucket": "other"}}
	owned := map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}
	require.NoError(t, CheckBackendOverrides("app", "dev", map[string]any{"configuration": map[string]any{}}, s3))
	require.NoError(t, CheckBackendOverrides("app", "dev", map[string]any{}, s3))
	require.NoError(t, CheckBackendOverrides("app", "dev", owned, BackendLayer{Type: "local"}, BackendLayer{}))

	err := CheckBackendOverrides("app", "dev", owned, BackendLayer{}, s3)
	require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
	require.ErrorContains(t, err, "backend_type")
	require.ErrorContains(t, err, `component "app" in stack "dev"`)

	inherited := BackendLayer{Type: "local", Settings: map[string]any{"local": map[string]any{"path": "inherited.tfstate"}}}
	require.ErrorContains(t, CheckBackendOverrides("app", "dev", owned, inherited), "backend.local")
}
