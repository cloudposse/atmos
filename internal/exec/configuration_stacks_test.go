package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_configuration_test.go -package=exec github.com/cloudposse/atmos/pkg/schema ComponentConfigurationRuntime

func TestConfigurationStacksUseStackCompiler(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), TerraformDirAbsolutePath: filepath.Join(root, "components")}
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config.ComponentConfigurations = provider
	definitions := map[string]map[string]any{"live": {"name": "live", "vars": map[string]any{"shared": "inherited"}, "components": map[string]any{"terraform": map[string]any{"dev/app": map[string]any{"source": "./modules/app", "vars": map[string]any{"own": "value"}}}}}}
	provider.EXPECT().Discover(gomock.Any()).Return(definitions, nil)
	stacks := map[string]any{}
	raw := map[string]map[string]any{}
	contexts := AllStacksDeferredContexts{}
	require.NoError(t, mergeConfigurationStacks(config, stacks, raw, contexts))
	component := stacks["live"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["dev/app"].(map[string]any)
	require.Equal(t, map[string]any{"shared": "inherited", "own": "value"}, component["vars"])
	require.Equal(t, "live", raw["live"]["name"])
}

func TestConfigurationStacksDoNotReuseAnotherInvocationsCache(t *testing.T) {
	root := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root, StacksBaseAbsolutePath: root, TerraformDirAbsolutePath: filepath.Join(root, "components")}
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true}}}
	// Seed the ordinary cache with an empty YAML inventory at the same location.
	stacks, _, _, err := FindStacksMap(config, false)
	require.NoError(t, err)
	require.Empty(t, stacks)
	for _, revision := range []string{"first", "second"} {
		provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
		config.ComponentConfigurations = provider
		definitions := map[string]map[string]any{"live": {"name": "live", "components": map[string]any{"terraform": map[string]any{"app": map[string]any{"source": "./module", "vars": map[string]any{"revision": revision}}}}}}
		provider.EXPECT().Discover(gomock.Any()).Return(definitions, nil).Times(2)
		stacks, _, _, err := FindStacksMap(config, false)
		require.NoError(t, err)
		app := stacks["live"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
		require.Equal(t, revision, app["vars"].(map[string]any)["revision"])
		// Validation must not scan atmos.yaml as a stack when discovery is the
		// only inventory source, even though the project directory exists.
		require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("not: [a stack manifest"), 0o600))
		require.NoError(t, ValidateStacks(config))
	}
}

func TestConfigurationStacksRejectCollisionsAndErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		existing map[string]any
		err      error
		want     error
	}{
		{name: "physical", existing: map[string]any{"live": map[string]any{}}, want: errUtils.ErrDuplicatePluginExtension},
		{name: "logical", existing: map[string]any{"deploy/dev": map[string]any{"name": "live"}}, want: errUtils.ErrDuplicatePluginExtension},
		{name: "canceled", existing: map[string]any{}, err: context.Canceled, want: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
			provider.EXPECT().Discover(gomock.Any()).Return(map[string]map[string]any{"live": {"name": "live"}}, test.err)
			err := mergeConfigurationStacks(&schema.AtmosConfiguration{ComponentConfigurations: provider}, test.existing, map[string]map[string]any{}, AllStacksDeferredContexts{})
			require.ErrorIs(t, err, test.want)
		})
	}
	require.NoError(t, mergeConfigurationStacks(&schema.AtmosConfiguration{}, nil, nil, nil))
	provider := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	provider.EXPECT().Discover(gomock.Any()).Return(nil, context.Canceled)
	stacks, raw, deferred, err := FindStacksMap(&schema.AtmosConfiguration{ComponentConfigurations: provider}, false)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, stacks)
	require.Nil(t, raw)
	require.Nil(t, deferred)
}
