package configuration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// directories creates project directories so that imports naming them exist.
func directories(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.FromSlash(name)), 0o755))
	}
}

func importOnlyConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	config := enabledConfig(t)
	directories(t, config.BasePathAbsolute, "live/app", "empty", "late")
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Discovery.Enabled = false
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	return config
}

func TestImportedUnitsBecomeComponentsOfTheImportingStack(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	network, app := unit(t, "network"), unit(t, "app")
	app.Dependencies = []*pluginv1.ComponentDependency{{Target: network.Address, Kind: pluginv1.ComponentDependency_OUTPUT}}
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Empty(t, req.IncludePaths, "imports are not limited by included_paths")
		require.Empty(t, req.ExcludePaths, "imports are not filtered by excluded_paths")
		require.Equal(t, []string{"terragrunt://live"}, req.Imports)
		require.Nil(t, req.Effects)
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network, app}}, nil
	})
	loads := 0
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(_ context.Context, names []string) (map[string]Provider, error) {
		loads++
		require.Equal(t, []string{"terragrunt"}, names)
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	require.Zero(t, loads, "no plugin starts before an import or discovery needs it")

	// The same import from two stacks (or a mixin) resolves once and yields independent copies.
	var first, second map[string]any
	for _, target := range []*map[string]any{&first, &second} {
		components, handled, err := session.ImportComponents(t.Context(), "terragrunt://live/.")
		require.NoError(t, err)
		require.True(t, handled)
		*target = components
	}
	require.Equal(t, 1, loads)
	require.Len(t, first, 2)
	appConfig := first["app"].(map[string]any)
	require.Equal(t, []any{map[string]any{"name": "network", "kind": "terraform"}}, appConfig["dependencies"].(map[string]any)["components"], "an imported unit depends on its siblings in the importing stack")
	require.Equal(t, "live/app/terragrunt.hcl", appConfig["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
	first["app"] = "corrupt"
	require.IsType(t, map[string]any{}, second["app"])

	// Import-only configuration has no derived stacks.
	definitions, err := session.Discover(t.Context())
	require.NoError(t, err)
	require.Empty(t, definitions)

	// Prepare finds the unit by source, under the stack and component the user selected.
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		require.Equal(t, "dev", req.Address.Stack)
		require.Equal(t, "web", req.Address.Component)
		require.Equal(t, "live/app/terragrunt.hcl", req.Source)
		return &pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", State: &pluginv1.ComponentStateIdentity{BackendType: "local"}}, nil
	})
	_, err = session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "web", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": "live/app/terragrunt.hcl"}}})
	require.NoError(t, err)
	_, err = session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "web", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": "live/other/terragrunt.hcl"}}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestImportedUnitsAreNotAlsoDiscoveredInADerivedStack(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		if len(req.Imports) > 0 {
			return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
		}
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app"), unit(t, "db")}}, nil
	}).Times(2)
	config := enabledConfig(t)
	directories(t, config.BasePathAbsolute, "live/app", "late")
	require.NoError(t, os.WriteFile(filepath.Join(config.BasePathAbsolute, "live", "app", "terragrunt.hcl"), nil, 0o644))
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://live/app/terragrunt.hcl")
	require.NoError(t, err)
	definitions, err := session.Discover(t.Context())
	require.NoError(t, err)
	derived := definitions["live"]["components"].(map[string]any)["terraform"].(map[string]any)
	require.NotContains(t, derived, "app", "an imported unit belongs to the importing stack only")
	require.Contains(t, derived, "db")
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://late")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, "imports after discovery would be silently ignored")
}

func TestImportOfUnitsWithTheSameNameFails(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	dev, prod := unit(t, "vpc"), unit(t, "vpc")
	dev.Source, prod.Source = "live/dev/vpc/terragrunt.hcl", "live/prod/vpc/terragrunt.hcl"
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{dev, prod}}, nil)
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://live")
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/dev/vpc/terragrunt.hcl")
	require.ErrorContains(t, err, "live/prod/vpc/terragrunt.hcl")
}

func TestImportOfAMissingPathNamesIt(t *testing.T) {
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		t.Fatal("a missing path must be rejected before the provider runs")
		return nil, nil
	})
	require.NoError(t, err)
	_, handled, err := session.ImportComponents(t.Context(), "terragrunt://live/absent")
	require.True(t, handled)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "terragrunt://live/absent")
}

func TestImportThatMatchesNothingFails(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{}, nil)
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://empty")
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportEmpty)
}

func TestImportValidationAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	session, err := New(ctx, enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) { return nil, nil })
	require.NoError(t, err)
	for _, uri := range []string{"ordinary", "https://example.com/config.yaml", "other://live"} {
		_, handled, err := session.ImportComponents(t.Context(), uri)
		require.NoError(t, err)
		require.False(t, handled)
	}
	for _, path := range []string{"", "../outside", "/absolute", `live\app`, "live?ref=main", "live#fragment", "live/*"} {
		_, handled, err := session.ImportComponents(t.Context(), "terragrunt://"+path)
		require.True(t, handled)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	canceled, stop := context.WithCancel(t.Context())
	stop()
	_, _, err = session.ImportComponents(canceled, "terragrunt://live")
	require.ErrorIs(t, err, context.Canceled)
	cancel()
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://live")
	require.ErrorIs(t, err, context.Canceled)
}

func TestConcurrentImportsResolveOnce(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil).Times(1)
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	var workers sync.WaitGroup
	for range 10 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, _, err := session.ImportComponents(t.Context(), "terragrunt://live"); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	var _ schema.ComponentConfigurationImportRuntime = session
}

func TestImportConflictsRejectOnlyDifferentUnits(t *testing.T) {
	unitComponent := func(source string) map[string]any {
		return map[string]any{"settings": map[string]any{"configuration": map[string]any{"source": source}}}
	}
	fragment := func(name string, component map[string]any) map[string]any {
		return ImportFragment(map[string]any{name: component})
	}
	yaml := map[string]any{"components": map[string]any{"terraform": map[string]any{"vpc": map[string]any{"vars": map[string]any{"a": 1}}}}}

	require.NoError(t, CheckImportConflicts("stacks/dev.yaml", nil))
	require.NoError(t, CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))}), "the same unit imported twice is one component")
	require.NoError(t, CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("app", unitComponent("live/app/terragrunt.hcl")), yaml}))

	err := CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("vpc", unitComponent("live/dev/vpc/terragrunt.hcl")), fragment("vpc", unitComponent("live/prod/vpc/terragrunt.hcl"))})
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/dev/vpc/terragrunt.hcl")
	require.ErrorContains(t, err, "live/prod/vpc/terragrunt.hcl")

	// A YAML component with the name of a unit is an override, wherever it comes from.
	for _, configs := range [][]map[string]any{
		{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), yaml},
		{yaml, fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))},
		{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), yaml, fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))},
	} {
		require.NoError(t, CheckImportConflicts("stacks/dev.yaml", configs))
	}
}

func TestOwnsComponentRecognizesProviderSections(t *testing.T) {
	owned := map[string]any{"settings": map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}}
	require.True(t, OwnsComponent(owned))
	require.False(t, OwnsComponent(map[string]any{"settings": map[string]any{"team": "platform"}}))
	require.False(t, OwnsComponent(nil))
}
