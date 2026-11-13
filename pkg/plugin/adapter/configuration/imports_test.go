package configuration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
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
	}).Times(2) // The import, then the dependency recheck that preparation runs under its own environment.
	// The recheck confirms the output dependency still reads the state it was discovered with.
	provider.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		require.Equal(t, network.Source, req.Source)
		return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{BackendType: "local"}}, nil
	}).Times(2)
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
	appConfig["settings"].(map[string]any)["configuration"].(map[string]any)["source"] = "corrupt"
	require.Equal(t, "live/app/terragrunt.hcl", second["app"].(map[string]any)["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
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
	for _, path := range []string{"", "../outside", "/absolute", `live\app`, "live?ref=main", "live#fragment", "live/*", "live/[ab]", "live/[dev]", "live/{a,b}"} {
		_, handled, err := session.ImportComponents(t.Context(), "terragrunt://"+path)
		require.True(t, handled)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		require.ErrorContains(t, err, "terragrunt://"+path)
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

func TestAliasedImportSchemeUsesTheProviderName(t *testing.T) {
	config := importOnlyConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Provider = "terragrunt"
	config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"tg": settings}
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Equal(t, "terragrunt", request.Provider)
		require.Equal(t, []string{"terragrunt://live"}, request.Imports, "the provider only recognizes its own name")
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
	})
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	for _, uri := range []string{"tg://live", "tg://live/."} {
		components, handled, err := session.ImportComponents(t.Context(), uri)
		require.NoError(t, err)
		require.True(t, handled)
		require.Contains(t, components, "app")
	}
}

func TestImportConflictsRejectOnlyDifferentUnits(t *testing.T) {
	unitComponent := func(source string) map[string]any {
		return map[string]any{"settings": map[string]any{"configuration": map[string]any{"source": source}}}
	}
	fragment := func(name string, component map[string]any) map[string]any {
		return configuration.ImportFragment(map[string]any{name: component})
	}
	yaml := map[string]any{"components": map[string]any{"terraform": map[string]any{"vpc": map[string]any{"vars": map[string]any{"a": 1}}}}}

	require.NoError(t, configuration.CheckImportConflicts("stacks/dev.yaml", nil))
	require.NoError(t, configuration.CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))}), "the same unit imported twice is one component")
	require.NoError(t, configuration.CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("app", unitComponent("live/app/terragrunt.hcl")), yaml}))

	err := configuration.CheckImportConflicts("stacks/dev.yaml", []map[string]any{fragment("vpc", unitComponent("live/dev/vpc/terragrunt.hcl")), fragment("vpc", unitComponent("live/prod/vpc/terragrunt.hcl"))})
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/dev/vpc/terragrunt.hcl")
	require.ErrorContains(t, err, "live/prod/vpc/terragrunt.hcl")

	// A YAML component with the name of a unit is an override, wherever it comes from.
	for _, configs := range [][]map[string]any{
		{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), yaml},
		{yaml, fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))},
		{fragment("vpc", unitComponent("live/vpc/terragrunt.hcl")), yaml, fragment("vpc", unitComponent("live/vpc/terragrunt.hcl"))},
	} {
		require.NoError(t, configuration.CheckImportConflicts("stacks/dev.yaml", configs))
	}
}

func TestOwnsComponentRecognizesProviderSections(t *testing.T) {
	owned := map[string]any{"settings": map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}}
	require.True(t, configuration.OwnsComponent(owned))
	require.False(t, configuration.OwnsComponent(map[string]any{"settings": map[string]any{"team": "platform"}}))
	require.False(t, configuration.OwnsComponent(nil))
}

// An explicit import of a configured provider whose settings are invalid reports the setting and
// its repair hint, instead of falling through to remote import resolution.
func TestImportOfAProviderWithInvalidSettingsReportsTheSetting(t *testing.T) {
	config := importOnlyConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Mode = "wrapper"
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		t.Fatal("invalid settings must be reported before the provider runs")
		return nil, nil
	})
	require.NoError(t, err)
	_, handled, err := session.ImportComponents(t.Context(), "terragrunt://live")
	require.True(t, handled)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, cockroachErrors.FlattenHints(err), "The only supported mode is `native`")
	// A scheme no section configures is still not handled here.
	_, handled, err = session.ImportComponents(t.Context(), "other://live")
	require.NoError(t, err)
	require.False(t, handled)
}

func TestValidationImportsDoNotChangeRunningDiscovery(t *testing.T) {
	config := enabledConfig(t)
	directories(t, config.BasePathAbsolute, "live/app")
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	started, finish := make(chan struct{}), make(chan struct{})
	app := unit(t, "app")
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		if len(req.Imports) == 0 {
			close(started)
			<-finish
		}
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{app}}, nil
	}).Times(2)
	session, err := New(t.Context(), config, "describe affected", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, err := session.Discover(t.Context()); done <- err }()
	<-started
	validation := session.ForValidation().(schema.ComponentConfigurationImportRuntime)
	var workers sync.WaitGroup
	for range 10 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			components, handled, err := validation.ImportComponents(t.Context(), "terragrunt://live/app")
			if err != nil || !handled || components["app"] == nil {
				t.Errorf("validation import: handled=%v, err=%v, components=%v", handled, err, components)
			}
		}()
	}
	workers.Wait()
	close(finish)
	require.NoError(t, <-done)
	definitions, err := session.Discover(t.Context())
	require.NoError(t, err)
	require.Contains(t, definitions["live"]["components"].(map[string]any)["terraform"], "app")
	require.Empty(t, session.imported, "validation must not register preparation sources")
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://live/app")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, "a validation cache entry does not authorize a late execution import")
	first, _, err := validation.ImportComponents(t.Context(), "terragrunt://live/app")
	require.NoError(t, err)
	first["app"].(map[string]any)["settings"] = "corrupt"
	again, _, err := validation.ImportComponents(t.Context(), "terragrunt://live/app")
	require.NoError(t, err)
	require.IsType(t, map[string]any{}, again["app"].(map[string]any)["settings"])
	revision, err := session.ForRevision(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, revision.(*Session).validationImports)
}

func TestImportInProgressCancellation(t *testing.T) {
	for _, phase := range []string{"loader", "discovery"} {
		for _, origin := range []string{"session", "caller", "deadline"} {
			t.Run(phase+"/"+origin, func(t *testing.T) {
				sessionCtx, cancelSession := context.WithCancel(t.Context())
				defer cancelSession()
				callerCtx, cancelCaller := context.WithCancel(t.Context())
				defer cancelCaller()
				// A generous caller deadline also verifies that composition retains the deadline.
				deadline := time.Now().Add(time.Minute)
				if origin == "deadline" {
					deadline = time.Now().Add(time.Second)
				}
				callerCtx, cancelDeadline := context.WithDeadline(callerCtx, deadline)
				defer cancelDeadline()
				started := make(chan context.Context, 1)
				block := func(ctx context.Context) error {
					started <- ctx
					<-ctx.Done()
					return ctx.Err()
				}
				provider := NewMockProvider(gomock.NewController(t))
				if phase == "discovery" {
					provider.EXPECT().ComponentType().Return("terraform")
					provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
						return nil, block(ctx)
					})
				}
				session, err := New(sessionCtx, importOnlyConfig(t), "list", func(ctx context.Context, _ []string) (map[string]Provider, error) {
					if phase == "loader" {
						return nil, block(ctx)
					}
					return map[string]Provider{"terragrunt": provider}, nil
				})
				require.NoError(t, err)
				done := make(chan error, 1)
				go func() {
					_, _, err := session.ImportComponents(callerCtx, "terragrunt://live")
					done <- err
				}()
				select {
				case ctx := <-started:
					got, ok := ctx.Deadline()
					require.True(t, ok)
					require.Equal(t, deadline, got)
				case <-time.After(5 * time.Second):
					t.Fatal("import did not reach provider")
				}
				switch origin {
				case "session":
					cancelSession()
				case "caller":
					cancelCaller()
				}
				select {
				case err := <-done:
					want := context.Canceled
					if origin == "deadline" {
						want = context.DeadlineExceeded
					}
					require.ErrorIs(t, err, want)
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not unblock import")
				}
				require.Empty(t, session.importResults, "failed imports must not be cached")
			})
		}
	}
}
