package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	componentadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	configurationadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

type importRuntime struct {
	*MockComponentConfigurationRuntime
	imports []string
	err     error
	// unit replaces the default component returned for "app".
	unit map[string]any
}

func (r *importRuntime) ImportComponents(_ context.Context, uri string) (map[string]any, bool, error) {
	r.imports = append(r.imports, uri)
	if r.unit != nil {
		return map[string]any{"app": r.unit}, true, r.err
	}
	return map[string]any{"app": map[string]any{"settings": map[string]any{"configuration": map[string]any{"source": "live/app/terragrunt.hcl"}}}}, true, r.err
}

// Imported units are merged into the manifest that imports them (and so into every stack that
// imports that manifest), exactly like components of an imported YAML mixin.
func TestYAMLImportsAddUnitsToTheImportingManifest(t *testing.T) {
	root := t.TempDir()
	mixin := filepath.Join(root, "mixin.yaml")
	require.NoError(t, os.WriteFile(mixin, []byte("import:\n  - terragrunt://live\n"), 0o600))
	path := filepath.Join(root, "adopt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("import:\n  - mixin\nvars:\n  stage: dev\n"), 0o600))
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t))}
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	result, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.NoError(t, err)
	require.Equal(t, []string{"terragrunt://live"}, runtime.imports)
	components := result.DeepMergedConfig["components"].(map[string]any)["terraform"].(map[string]any)
	require.Contains(t, components, "app")
}

// placeholderUnit mirrors what a provider discovers: placeholder sections that the stack may override.
func placeholderUnit() map[string]any {
	return map[string]any{
		"vars":     map[string]any{},
		"metadata": map[string]any{"terraform_workspace": "default"},
		"settings": map[string]any{"configuration": map[string]any{"source": "live/app/terragrunt.hcl", "provider": "terragrunt"}},
	}
}

func processUnitManifest(t *testing.T, root, name, content string, runtime *importRuntime) map[string]any {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	result, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.NoError(t, err)
	components := result.DeepMergedConfig["components"].(map[string]any)["terraform"].(map[string]any)
	app, ok := components["app"].(map[string]any)
	require.True(t, ok)
	return app
}

// A YAML component named like an imported unit is an override that deep-merges over the unit,
// whether it is written in the importing manifest or in an import that comes before the unit.
func TestYAMLComponentOverridesAnImportedUnit(t *testing.T) {
	root := t.TempDir()
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), unit: placeholderUnit()}

	app := processUnitManifest(t, root, "adopt.yaml", "import:\n  - terragrunt://live\ncomponents:\n  terraform:\n    app:\n      vars:\n        a: 1\n        nested:\n          key: value\n      settings:\n        team: platform\n      metadata:\n        terraform_workspace: blue\n", runtime)
	require.Equal(t, map[string]any{"a": 1, "nested": map[string]any{"key": "value"}}, app["vars"])
	settings := app["settings"].(map[string]any)
	require.Equal(t, "platform", settings["team"])
	require.Equal(t, "live/app/terragrunt.hcl", settings["configuration"].(map[string]any)["source"], "the unit marker survives a YAML settings block")
	require.Equal(t, "blue", app["metadata"].(map[string]any)["terraform_workspace"])

	// The override sits in an import that precedes the unit's import: the unit is still the base layer.
	require.NoError(t, os.WriteFile(filepath.Join(root, "catalog.yaml"), []byte("components:\n  terraform:\n    app:\n      vars:\n        from_catalog: true\n      metadata:\n        terraform_workspace: green\n"), 0o600))
	app = processUnitManifest(t, root, "early.yaml", "import:\n  - catalog\n  - terragrunt://live\n", runtime)
	require.Equal(t, map[string]any{"from_catalog": true}, app["vars"])
	require.Equal(t, "green", app["metadata"].(map[string]any)["terraform_workspace"], "an earlier YAML override must not lose to the unit's placeholder")
	require.Equal(t, "live/app/terragrunt.hcl", app["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
}

func TestNestedUnitImportsPreserveYAMLPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		imports string
		mixin   string
		want    string
	}{
		{"catalog before nested unit", "catalog, outer", "import: [terragrunt://live]\n", "green"},
		{"catalog after nested unit", "outer, catalog", "import: [terragrunt://live]\n", "green"},
		{"explicit mixin workspace wins", "catalog, outer", "import: [terragrunt://live]\ncomponents:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace: blue\n", "blue"},
		{"explicit default remains YAML", "catalog, outer", "import: [terragrunt://live]\ncomponents:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace: default\n", "default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for name, body := range map[string]string{
				"catalog": "components:\n  terraform:\n    app:\n      metadata:\n        terraform_workspace: green\n      vars:\n        from_catalog: true\n",
				"mixin":   tc.mixin,
				"outer":   "import: [mixin]\n",
			} {
				require.NoError(t, os.WriteFile(filepath.Join(root, name+".yaml"), []byte(body), 0o600))
			}
			unit := placeholderUnit()
			runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), unit: unit}
			app := processUnitManifest(t, root, "stack.yaml", "import: ["+tc.imports+"]\n", runtime)
			require.Equal(t, tc.want, app["metadata"].(map[string]any)["terraform_workspace"])
			require.Equal(t, true, app["vars"].(map[string]any)["from_catalog"])
			require.Equal(t, "live/app/terragrunt.hcl", app["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
			require.Equal(t, placeholderUnit(), unit, "loading must not mutate provider data")
		})
	}
}

func TestTwoUnitsWithOneComponentNameStillConflict(t *testing.T) {
	root := t.TempDir()
	first := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), unit: placeholderUnit()}
	second := placeholderUnit()
	second["settings"].(map[string]any)["configuration"].(map[string]any)["source"] = "live/other/terragrunt.hcl"
	path := filepath.Join(root, "adopt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("import:\n  - terragrunt://live\n  - terragrunt://other\n"), 0o600))
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: &alternatingRuntime{importRuntime: first, units: []map[string]any{first.unit, second}}}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	_, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/app/terragrunt.hcl")
	require.ErrorContains(t, err, "live/other/terragrunt.hcl")
}

// alternatingRuntime returns a different unit for every import, in order.
type alternatingRuntime struct {
	*importRuntime
	units []map[string]any
	calls int
}

func (r *alternatingRuntime) ImportComponents(_ context.Context, _ string) (map[string]any, bool, error) {
	unit := r.units[r.calls%len(r.units)]
	r.calls++
	return map[string]any{"app": unit}, true, nil
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
	again, err := configurationWatchIndex(config, settings, index)
	require.NoError(t, err)
	require.Same(t, filtered, again, "components sharing exclusions must reuse one filtered index")
	other, err := configurationWatchIndex(config, map[string]any{"configuration": map[string]any{"watch_excludes": []any{"**/*.yaml"}}}, index)
	require.NoError(t, err)
	require.NotSame(t, filtered, other)
	again, err = configurationWatchIndex(config, settings, index)
	require.NoError(t, err)
	require.Same(t, filtered, again, "alternating exclusion sets must reuse their views")
	independent := newChangedFilesIndex(config, index.getAllFiles(), root)
	independentView, err := configurationWatchIndex(config, settings, independent)
	require.NoError(t, err)
	require.NotSame(t, filtered, independentView, "separate change sets must own their views")
	require.Equal(t, []string{filepath.Join(root, "live", ".terragrunt-cache", "generated.tf")}, other.getAllFiles())
	unchanged, err := configurationWatchIndex(config, nil, index)
	require.NoError(t, err)
	require.Same(t, index, unchanged)
	empty, err := configurationWatchIndex(config, map[string]any{"configuration": map[string]any{"watch_excludes": []any{}}}, index)
	require.NoError(t, err)
	require.Same(t, index, empty)
	_, err = configurationWatchIndex(config, map[string]any{"configuration": map[string]any{"watch_excludes": "**/.terragrunt-cache/**"}}, index)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
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
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
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

// Two mixins that import different units with one component name conflict in the manifest that
// imports both, even though that manifest has no provider import of its own.
func TestUnitsImportedThroughDifferentMixinsConflict(t *testing.T) {
	root := t.TempDir()
	first := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), unit: placeholderUnit()}
	second := placeholderUnit()
	second["settings"].(map[string]any)["configuration"].(map[string]any)["source"] = "live/other/terragrunt.hcl"
	require.NoError(t, os.WriteFile(filepath.Join(root, "first.yaml"), []byte("import:\n  - terragrunt://live\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "second.yaml"), []byte("import:\n  - terragrunt://other\n"), 0o600))
	path := filepath.Join(root, "adopt.yaml")
	require.NoError(t, os.WriteFile(path, []byte("import:\n  - first\n  - second\n"), 0o600))
	config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: &alternatingRuntime{importRuntime: first, units: []map[string]any{first.unit, second}}}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
	_, err := ProcessYAMLConfigFile(config, root, path, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/app/terragrunt.hcl")
	require.ErrorContains(t, err, "live/other/terragrunt.hcl")
}

// A manifest's Terraform overrides reach the units it imports, directly or through a mixin, without
// changing the provider's import result.
func TestStackOverridesApplyToImportedUnits(t *testing.T) {
	root := t.TempDir()
	unit := placeholderUnit()
	runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t)), unit: unit}
	overrides := "terraform:\n  overrides:\n    vars:\n      region: us-west-2\n"

	app := processUnitManifest(t, root, "direct.yaml", "import:\n  - terragrunt://live\n"+overrides, runtime)
	require.Equal(t, map[string]any{"vars": map[string]any{"region": "us-west-2"}}, app["overrides"])
	require.NotContains(t, unit, "overrides", "the provider's unit must not be changed")

	require.NoError(t, os.WriteFile(filepath.Join(root, "mixin.yaml"), []byte("import:\n  - terragrunt://live\n"), 0o600))
	app = processUnitManifest(t, root, "nested.yaml", "import:\n  - mixin\n"+overrides, runtime)
	require.Equal(t, map[string]any{"vars": map[string]any{"region": "us-west-2"}}, app["overrides"])
}

func TestStaticLoadsRejectProviderImportsBeforeRuntime(t *testing.T) {
	for _, nested := range []bool{false, true} {
		name := "direct"
		if nested {
			name = "nested"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "dev.yaml")
			imports := "terragrunt://live"
			if nested {
				require.NoError(t, os.WriteFile(filepath.Join(root, "mixin.yaml"), []byte("import: [terragrunt://live]\n"), 0o600))
				imports = "mixin"
			}
			require.NoError(t, os.WriteFile(file, []byte("name: dev\nimport: ["+imports+"]\n"), 0o600))
			runtime := &importRuntime{MockComponentConfigurationRuntime: NewMockComponentConfigurationRuntime(gomock.NewController(t))}
			config := &schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: root, StackConfigFilesAbsolutePaths: []string{file}, ComponentConfigurations: runtime}
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
			_, err := ExecuteDescribeStacksWithOptions(config, "dev", nil, nil, nil,
				false, false, false, true, nil, nil, false, DescribeStacksErrorOptions{StaticOnly: true})
			require.ErrorIs(t, err, errUtils.ErrInvalidImport)
			require.ErrorContains(t, err, "cannot inspect provider import")
			require.Empty(t, runtime.imports, "static inspection must never invoke the provider runtime")

			// The same manifest still imports units during an ordinary load.
			result, err := ProcessYAMLConfigFile(config, root, file, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
			require.NoError(t, err)
			require.Equal(t, []string{"terragrunt://live"}, runtime.imports)
			require.Contains(t, result.DeepMergedConfig["components"].(map[string]any)["terraform"], "app")

			// Even a previously used runtime must not serve a later static load.
			_, handled, err := importConfigurationComponents(config, "terragrunt://live", &schema.StackImport{}, &StackLoadOptions{StaticOnly: true})
			require.True(t, handled)
			require.ErrorIs(t, err, errUtils.ErrInvalidImport)
			require.Len(t, runtime.imports, 1)
		})
	}
}

// validationImportProvider returns a unit both through discovery and through an unreferenced mixin.
type validationImportProvider struct {
	configurationadapter.Provider
	imports int
}

func (*validationImportProvider) ComponentType() string { return "terraform" }

func (p *validationImportProvider) Discover(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	if len(request.Imports) > 0 {
		p.imports++
	}
	return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{{
		Address:  &pluginv1.ComponentAddress{Stack: "live", Component: "app", ComponentType: "terraform"},
		Provider: "terragrunt", Engine: "opentofu", Source: "live/app/terragrunt.hcl",
	}}}, nil
}

func TestValidateStacksResolvesUnreferencedProviderImports(t *testing.T) {
	for _, path := range []string{"live/app", "missing"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			stacks := filepath.Join(root, "stacks")
			require.NoError(t, os.MkdirAll(stacks, 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "live", "app"), 0o755))
			mixin := filepath.Join(stacks, "mixin.yaml")
			require.NoError(t, os.WriteFile(mixin, []byte("import:\n  - terragrunt://"+path+"\n"), 0o600))
			config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, StacksBaseAbsolutePath: stacks, TerraformDirAbsolutePath: filepath.Join(root, "components")}
			config.Stacks.BasePath = "stacks"
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true, IncludedPaths: []string{"live/**"}}}}
			provider := &validationImportProvider{}
			session, err := configurationadapter.New(t.Context(), config, "describe affected", func(context.Context, []string) (map[string]configurationadapter.Provider, error) {
				return map[string]configurationadapter.Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			config.ComponentConfigurations = session
			// No stack manifest includes the mixin. Validation must still read it after discovery,
			// as it does before describe affected compares the HEAD and target inventories.
			before, err := session.Discover(t.Context())
			require.NoError(t, err)
			if path == "missing" {
				require.ErrorContains(t, ValidateStacks(config), "does not exist in the project")
				return
			}
			require.NoError(t, ValidateStacks(config))
			require.NoError(t, ValidateStacks(config))
			require.Equal(t, 1, provider.imports, "both validation passes and repeat validation share the import cache")
			require.Same(t, session, config.ComponentConfigurations)
			after, err := session.Discover(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after, "validation must not remove the discovered unit")
		})
	}
}

// Exercise real provider import translation, YAML merging, stack compilation and preparation.
func TestImportedUnitBackendOptionsBeforeNormalization(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		rejected   bool
	}{
		{"component local", "components:\n  terraform:\n    app:\n      backend:\n        local:\n          path: other.tfstate\n", true},
		{"component backend type", "components:\n  terraform:\n    app:\n      backend_type: s3\n      backend:\n        s3:\n          bucket: other\n", true},
		{"inherited local", "components:\n  terraform:\n    base:\n      metadata:\n        type: abstract\n      backend:\n        local:\n          path: inherited.tfstate\n    app:\n      metadata:\n        inherits: [base]\n", true},
		{"global local", "terraform:\n  backend_type: local\n  backend:\n    local:\n      path: global.tfstate\n", false},
		{"global s3", "terraform:\n  backend_type: s3\n  backend:\n    s3:\n      bucket: global\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "live", "app"), 0o755))
			file := filepath.Join(root, "dev.yaml")
			require.NoError(t, os.WriteFile(file, []byte("import: [terragrunt://live/app]\n"+tc.yaml), 0o600))
			config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, StacksBaseAbsolutePath: root}
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
			provider := &backendOptionsProvider{}
			placeholder, err := componentadapter.EncodeObject(map[string]any{"backend_type": "local", "backend": map[string]any{"local": map[string]any{}}})
			require.NoError(t, err)
			provider.configuration = placeholder
			t.Cleanup(func() {
				want := 1
				if tc.rejected {
					want = 0
				}
				require.Equal(t, want, provider.prepares)
			})
			session, err := configurationadapter.New(t.Context(), config, "plan", func(context.Context, []string) (map[string]configurationadapter.Provider, error) {
				return map[string]configurationadapter.Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			config.ComponentConfigurations = session
			loaded, err := ProcessYAMLConfigFile(config, root, file, map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
			require.NoError(t, err)
			compiled, _, err := ProcessStackConfig(config, root, "", "", "", "", file, loaded.DeepMergedConfig, false, false, "", nil, loaded.ImportsConfig, false)
			if tc.rejected {
				require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
				require.ErrorContains(t, err, "app")
				require.ErrorContains(t, err, "dev")
				return // The cleanup assertion verifies rejection before provider effects.
			}
			require.NoError(t, err)
			app := compiled["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
			backend, err := componentadapter.EncodeObject(map[string]any{"path": "unit.tfstate"})
			require.NoError(t, err)
			provider.response = &pluginv1.PrepareComponentResponse{Engine: "terraform", State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, WorkingDirectory: root}}
			prepared, err := session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "app", SubCommand: "plan", ComponentSection: app, ComponentSettingsSection: app["settings"].(map[string]any)})
			require.NoError(t, err)
			require.Equal(t, filepath.Join(root, "unit.tfstate"), prepared.Backend["path"])
		})
	}
}

type backendOptionsProvider struct {
	configurationadapter.Provider
	configuration *pluginv1.ComponentValue
	response      *pluginv1.PrepareComponentResponse
	prepares      int
}

func (*backendOptionsProvider) ComponentType() string { return "terraform" }
func (p *backendOptionsProvider) Discover(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{{Address: &pluginv1.ComponentAddress{Stack: "live", Component: "app", ComponentType: "terraform"}, Provider: "terragrunt", Engine: "terraform", Source: "live/app/terragrunt.hcl", Configuration: p.configuration}}}, nil
}

func (p *backendOptionsProvider) Prepare(context.Context, *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
	p.prepares++
	return p.response, nil
}

type provenanceImportRuntime struct {
	*MockComponentConfigurationRuntime
}

func (r *provenanceImportRuntime) ImportComponents(_ context.Context, uri string) (map[string]any, bool, error) {
	return map[string]any{"app": map[string]any{"settings": map[string]any{"configuration": map[string]any{"source": uri}}}}, true, nil
}

func TestConfigurationImportProvenance(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		conflict            bool
		source              string
	}{
		{name: "ordinary YAML source settings", first: "", second: "", source: ""},
		{name: "same unit with different YAML overrides", first: "terragrunt://same", second: "terragrunt://same", source: "terragrunt://same"},
		{name: "distinct units hidden by YAML overrides", first: "terragrunt://first", second: "terragrunt://second", conflict: true},
		{name: "unit and YAML source setting", first: "terragrunt://first", second: "", source: "terragrunt://first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, content string) {
				require.NoError(t, os.WriteFile(filepath.Join(root, name+".yaml"), []byte(content), 0o600))
			}
			for i, unit := range []string{tc.first, tc.second} {
				name := []string{"first", "second"}[i]
				content := ""
				if unit != "" {
					content = "import:\n  - " + unit + "\n"
				}
				// Distinct units get the same YAML source to ensure merging cannot hide the conflict.
				yamlSource := name
				if tc.conflict {
					yamlSource = "same-override"
				}
				write(name, content+"components:\n  terraform:\n    app:\n      settings:\n        configuration:\n          source: "+yamlSource+"\n")
				write(name+"-nested", "import:\n  - "+name+"\n")
			}
			write("stack", "import:\n  - first-nested\n  - second-nested\n")
			runtime := &provenanceImportRuntime{NewMockComponentConfigurationRuntime(gomock.NewController(t))}
			config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
			result, err := ProcessYAMLConfigFile(config, root, filepath.Join(root, "stack.yaml"), map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
			if tc.conflict {
				require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
				return
			}
			require.NoError(t, err)
			if tc.source == "" {
				require.Empty(t, result.ImportedUnitSources)
			} else {
				require.Equal(t, map[string]string{"app": tc.source}, result.ImportedUnitSources)
			}
			app := result.DeepMergedConfig["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
			require.Equal(t, "second", app["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
		})
	}
}

// Only a provider owns the backend; an empty `settings.configuration` map leaves ordinary components alone.
func TestBackendRestrictionRequiresProviderOwnership(t *testing.T) {
	opts := &ComponentProcessorOptions{Component: "app", StackName: "dev"}
	result := &ComponentProcessorResult{ComponentBackendType: "s3", ComponentBackendSection: map[string]any{"bucket": "other"}}
	require.NoError(t, checkConfigurationBackendOverrides(opts, result, map[string]any{"configuration": map[string]any{}}))
	require.NoError(t, checkConfigurationBackendOverrides(opts, result, map[string]any{}))
	err := checkConfigurationBackendOverrides(opts, result, map[string]any{"configuration": map[string]any{"provider": "terragrunt"}})
	require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
	require.ErrorContains(t, err, "backend_type")
}
