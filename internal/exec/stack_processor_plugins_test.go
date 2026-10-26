package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func processPluginTestStack(t *testing.T, atmosConfig *schema.AtmosConfiguration, config map[string]any) map[string]any {
	t.Helper()
	// Inheritance is cached by stack name, so each case needs its own stack identity.
	dir := t.TempDir()
	result, _, err := ProcessStackConfig(
		atmosConfig, dir, "", "", "", "", filepath.Join(dir, "plugins.yaml"), config,
		false, false, "", map[string]map[string][]string{}, map[string]map[string]any{}, false,
	)
	require.NoError(t, err)
	components, ok := result[cfg.ComponentsSectionName].(map[string]any)
	require.True(t, ok)
	return components
}

func TestProcessStackConfig_PluginDefaults(t *testing.T) {
	defaults := []any{"diff@v3.15.10"}
	base := []any{"secrets"}
	component := []any{"unittest", "s3"}
	tests := []struct {
		name      string
		defaults  any
		base      any
		component any
		strategy  string
		want      any
	}{
		{name: "absent"},
		{name: "defaults only", defaults: defaults, want: defaults},
		{name: "base replaces defaults", defaults: defaults, base: base, want: base},
		{name: "component replaces defaults", defaults: defaults, component: component, want: component},
		{name: "component replaces base", defaults: defaults, base: base, component: component, want: component},
		{name: "existing base inheritance", base: base, want: base},
		{name: "existing component plugins", component: component, want: component},
		{name: "empty defaults", defaults: []any{}, want: []any{}},
		{name: "empty base clears defaults", defaults: defaults, base: []any{}, want: []any{}},
		{name: "empty component clears inherited", defaults: defaults, base: base, component: []any{}, want: []any{}},
		{name: "explicit replace", defaults: defaults, base: base, component: component, strategy: "replace", want: component},
		{name: "append", defaults: defaults, base: base, component: component, strategy: "append", want: []any{"diff@v3.15.10", "secrets", "unittest", "s3"}},
		{name: "append empty preserves inherited", defaults: defaults, component: []any{}, strategy: "append", want: defaults},
		{name: "merge preserves scalar positions", defaults: defaults, base: base, component: component, strategy: "merge", want: []any{"diff@v3.15.10", "s3"}},
		{name: "merge empty preserves inherited", defaults: defaults, component: []any{}, strategy: "merge", want: defaults},
	}

	for _, componentType := range []string{cfg.HelmfileComponentType, cfg.HelmComponentType} {
		for _, tt := range tests {
			t.Run(componentType+"/"+tt.name, func(t *testing.T) {
				baseConfig := map[string]any{"metadata": map[string]any{"type": "abstract"}}
				componentConfig := map[string]any{"metadata": map[string]any{"inherits": []any{"base"}}}
				typeDefaults := map[string]any{}
				for _, section := range []struct {
					config map[string]any
					value  any
				}{{typeDefaults, tt.defaults}, {baseConfig, tt.base}, {componentConfig, tt.component}} {
					if section.value != nil {
						section.config[cfg.PluginsSectionName] = section.value
					}
				}
				// Component settings must override the atmos.yaml strategy.
				if tt.strategy != "" {
					componentConfig["settings"] = map[string]any{"list_merge_strategy": tt.strategy}
				}
				atmosConfig := &schema.AtmosConfiguration{Settings: schema.AtmosSettings{ListMergeStrategy: "replace"}}
				components := processPluginTestStack(t, atmosConfig, map[string]any{
					componentType: typeDefaults,
					"components": map[string]any{componentType: map[string]any{
						"base": baseConfig, "app": componentConfig, "other": map[string]any{},
					}},
				})
				resolved := components[componentType].(map[string]any)
				app := resolved["app"].(map[string]any)
				assert.Equal(t, tt.want, app[cfg.PluginsSectionName])
				if tt.want == nil {
					assert.NotContains(t, app, cfg.PluginsSectionName)
				}
				assert.Equal(t, tt.defaults, resolved["other"].(map[string]any)[cfg.PluginsSectionName])
				assert.Equal(t, "replace", atmosConfig.Settings.ListMergeStrategy, "component settings must not mutate shared config")
			})
		}
	}
}

func TestProcessStackConfig_PluginDefaultsFromImports(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "defaults.yaml"), []byte(`helmfile:
  plugins:
    - diff@v3.15.10
helm:
  plugins:
    - secrets
components:
  helmfile:
    imported: {}
  helm:
    imported: {}
`), 0o600))
	stackPath := filepath.Join(dir, "dev.yaml")
	require.NoError(t, os.WriteFile(stackPath, []byte(`import:
  - defaults
components:
  helmfile:
    local: {}
  helm:
    local: {}
  terraform:
    local: {}
`), 0o600))
	atmosConfig := &schema.AtmosConfiguration{}
	loaded, err := ProcessYAMLConfigFile(
		atmosConfig, dir, stackPath, map[string]map[string]any{}, nil,
		false, false, false, false, nil, nil, nil, nil, "",
	)
	require.NoError(t, err)
	components := processPluginTestStack(t, atmosConfig, loaded.DeepMergedConfig)
	for componentType, want := range map[string][]any{
		cfg.HelmfileComponentType: {"diff@v3.15.10"},
		cfg.HelmComponentType:     {"secrets"},
	} {
		resolved := components[componentType].(map[string]any)
		for _, name := range []string{"imported", "local"} {
			assert.Equal(t, want, resolved[name].(map[string]any)[cfg.PluginsSectionName], componentType+"/"+name)
		}
	}
	terraform := components[cfg.TerraformComponentType].(map[string]any)
	assert.NotContains(t, terraform["local"].(map[string]any), cfg.PluginsSectionName)
}
