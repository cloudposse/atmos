package configuration

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
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

// A discovered unit and an imported unit with another source that claim one stack and component
// are a conflict: preparation names both sources instead of running either unit.
func TestDiscoveredAndImportedUnitAtOneAddressConflict(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	catalog := unit(t, "db")
	catalog.Source = "catalog/db/terragrunt.hcl"
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		if len(req.Imports) > 0 {
			return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{catalog}}, nil
		}
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app"), unit(t, "db")}}, nil
	}).Times(2)
	config := enabledConfig(t)
	directories(t, config.BasePathAbsolute, "catalog")
	session, err := New(t.Context(), config, "plan", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	_, _, err = session.ImportComponents(t.Context(), "terragrunt://catalog")
	require.NoError(t, err)
	_, err = session.Discover(t.Context())
	require.NoError(t, err)

	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "db", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": catalog.Source}}}
	_, err = session.Prepare(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
	require.ErrorContains(t, err, "live/db/terragrunt.hcl")
	require.ErrorContains(t, err, "catalog/db/terragrunt.hcl")

	// Without a competing import, the discovered unit is still found at its address.
	info.ComponentSettingsSection = map[string]any{"configuration": map[string]any{"source": "live/db/terragrunt.hcl"}}
	instance, err := session.instanceFor(info)
	require.NoError(t, err)
	require.Equal(t, "live/db/terragrunt.hcl", instance.Source)
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
	require.Contains(t, cockroachErrors.FlattenHints(err), "The supported modes are `native` and `executable`")
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

// An import waiting for another import of the same URI gives up when its context ends, and never
// returns the other import's cached units after its own cancellation.
func TestCanceledImportWaiterDoesNotReturnCachedSuccess(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	started, release := make(chan struct{}), make(chan struct{})
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		close(started)
		<-release
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
	})
	session, err := New(t.Context(), importOnlyConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	first := make(chan error, 1)
	go func() {
		_, _, err := session.ImportComponents(t.Context(), "terragrunt://live")
		first <- err
	}()
	<-started

	waiter, cancel := context.WithCancel(t.Context())
	second := make(chan error, 1)
	go func() {
		_, _, err := session.ImportComponents(waiter, "terragrunt://live")
		second <- err
	}()
	cancel()
	select {
	case err := <-second:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("a canceled waiter must not wait for the in-flight import")
	}
	close(release)
	require.NoError(t, <-first)

	// A later caller with a live context still shares the cached result.
	components, handled, err := session.ImportComponents(t.Context(), "terragrunt://live")
	require.NoError(t, err)
	require.True(t, handled)
	require.Contains(t, components, "app")
}

// An import path that exists but cannot be inspected keeps its filesystem cause; only a missing
// path gets the "does not exist" diagnostic.
func TestImportPathFilesystemFailuresKeepTheirCause(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not block path inspection on Windows")
	}
	config := importOnlyConfig(t)
	locked := filepath.Join(config.BasePathAbsolute, "locked")
	require.NoError(t, os.MkdirAll(filepath.Join(locked, "unit"), 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := os.Stat(filepath.Join(locked, "unit")); err == nil {
		t.Skip("the process bypasses directory permissions")
	}
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		t.Fatal("an unreadable import path must be rejected before the provider runs")
		return nil, nil
	})
	require.NoError(t, err)
	_, handled, err := session.ImportComponents(t.Context(), "terragrunt://locked/unit")
	require.True(t, handled)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, err, fs.ErrPermission)
	require.NotContains(t, err.Error(), "does not exist")

	_, _, err = session.ImportComponents(t.Context(), "terragrunt://absent")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "does not exist")
}

// A unit imported on its own may depend on a unit beyond the imported root. The target is absent from
// the baseline and from the recheck, so preparation accepts it and still rejects a changed edge.
func TestImportedUnitDependencyOutsideTheImportedRoot(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    pluginv1.ComponentDependency_Kind
		changed bool
		want    error
	}{
		{name: "order dependency", kind: pluginv1.ComponentDependency_ORDER},
		{name: "output dependency", kind: pluginv1.ComponentDependency_OUTPUT},
		{name: "changed dependency target", kind: pluginv1.ComponentDependency_ORDER, changed: true, want: errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
			app := unit(t, "app")
			app.Dependencies = []*pluginv1.ComponentDependency{{Target: unit(t, "shared").Address, Kind: test.kind}}
			recheck := proto.Clone(app).(*pluginv1.DiscoveredComponent)
			if test.changed {
				recheck.Dependencies[0].Target = unit(t, "other").Address
			}
			calls := 0
			provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
				calls++
				if calls == 1 {
					return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{app}}, nil
				}
				return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{recheck}}, nil
			}).Times(2)
			if test.want == nil {
				provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", State: &pluginv1.ComponentStateIdentity{BackendType: "local"}}, nil)
			}
			session, err := New(t.Context(), importOnlyConfig(t), "plan", func(context.Context, []string) (map[string]Provider, error) {
				return map[string]Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			_, _, err = session.ImportComponents(t.Context(), "terragrunt://live/app")
			require.NoError(t, err)
			_, err = session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "web", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": app.Source}}})
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				return
			}
			require.NoError(t, err)
		})
	}
}

// An imported dependency's prepared state is stored under the importing stack, so the dependent
// finds it there instead of re-resolving the dependency under the discovery environment.
func TestImportedDependencyUsesItsPreparedStateInTheImportingStack(t *testing.T) {
	stateAt := func(path string) *pluginv1.ComponentStateIdentity {
		backend, err := adapter.EncodeObject(map[string]any{"path": path})
		require.NoError(t, err)
		return &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "default", WorkingDirectory: "/work"}
	}
	for _, test := range []struct {
		name      string
		dependent *pluginv1.ComponentStateIdentity
		want      error
	}{
		{name: "matches prepared dependency", dependent: stateAt("network.tfstate")},
		{name: "differs from prepared dependency", dependent: stateAt("other.tfstate"), want: errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
			network, app := unit(t, "network"), unit(t, "app")
			app.Dependencies = []*pluginv1.ComponentDependency{{Target: network.Address, Kind: pluginv1.ComponentDependency_OUTPUT}}
			provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network, app}}, nil).AnyTimes()
			provider.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
				require.Equal(t, "dependent", request.Invocation.Environment["IDENTITY"], "a prepared dependency must not be re-resolved")
				return &pluginv1.ResolveComponentStateResponse{State: test.dependent}, nil
			}).MinTimes(1)
			provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", State: stateAt("network.tfstate")}, nil)
			if test.want == nil {
				provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", State: stateAt("app.tfstate")}, nil)
			}
			session, err := New(t.Context(), importOnlyConfig(t), "apply", func(context.Context, []string) (map[string]Provider, error) {
				return map[string]Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			_, _, err = session.ImportComponents(t.Context(), "terragrunt://live")
			require.NoError(t, err)
			info := func(component string, unit *pluginv1.DiscoveredComponent, identity string) *schema.ConfigAndStacksInfo {
				return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: component, SubCommand: "apply", TerraformWorkspace: "default", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": unit.Source}}, SanitizedEnv: []string{"IDENTITY=" + identity}}
			}
			_, err = session.Prepare(t.Context(), info("network", network, "network"))
			require.NoError(t, err)
			_, err = session.Prepare(t.Context(), info("app", app, "dependent"))
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				return
			}
			require.NoError(t, err)
		})
	}
}

// An output dependency whose target was evaluated by a separate import is still checked: the
// dependent's state under its own environment must equal the target's prepared state. A target that
// no import evaluated has no state evidence and is skipped.
func TestDependencyPreparedStateIsComparedAcrossSeparateImports(t *testing.T) {
	stateAt := func(path string) *pluginv1.ComponentStateIdentity {
		backend, err := adapter.EncodeObject(map[string]any{"path": path})
		require.NoError(t, err)
		return &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "default", WorkingDirectory: "/work"}
	}
	for _, test := range []struct {
		name      string
		imports   []string
		dependent *pluginv1.ComponentStateIdentity
		want      error
	}{
		{name: "matches prepared dependency", imports: []string{"network", "app"}, dependent: stateAt("network.tfstate")},
		{name: "differs from prepared dependency", imports: []string{"network", "app"}, dependent: stateAt("other.tfstate"), want: errUtils.ErrPluginInvalidConfig},
		{name: "unimported target is skipped", imports: []string{"app"}, dependent: stateAt("other.tfstate")},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
			network, app := unit(t, "network"), unit(t, "app")
			app.Dependencies = []*pluginv1.ComponentDependency{{Target: network.Address, Kind: pluginv1.ComponentDependency_OUTPUT}}
			provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
				require.Len(t, request.Imports, 1)
				if request.Imports[0] == "terragrunt://live/network" {
					return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network}}, nil
				}
				return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{app}}, nil
			}).AnyTimes()
			resolved := 0
			provider.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
				resolved++
				require.Equal(t, "dependent", request.Invocation.Environment["IDENTITY"], "a prepared dependency must not be re-resolved")
				return &pluginv1.ResolveComponentStateResponse{State: test.dependent}, nil
			}).AnyTimes()
			provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", State: stateAt("network.tfstate")}, nil).AnyTimes()
			config := importOnlyConfig(t)
			directories(t, config.BasePathAbsolute, "live/network")
			session, err := New(t.Context(), config, "apply", func(context.Context, []string) (map[string]Provider, error) {
				return map[string]Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			for _, name := range test.imports {
				_, _, err = session.ImportComponents(t.Context(), "terragrunt://live/"+name)
				require.NoError(t, err)
			}
			info := func(component string, unit *pluginv1.DiscoveredComponent, identity string) *schema.ConfigAndStacksInfo {
				return &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: component, SubCommand: "apply", TerraformWorkspace: "default", ComponentSettingsSection: map[string]any{"configuration": map[string]any{"source": unit.Source}}, SanitizedEnv: []string{"IDENTITY=" + identity}}
			}
			if len(test.imports) > 1 {
				_, err = session.Prepare(t.Context(), info("network", network, "network"))
				require.NoError(t, err)
			}
			_, err = session.Prepare(t.Context(), info("app", app, "dependent"))
			if test.want != nil {
				require.ErrorIs(t, err, test.want)
				return
			}
			require.NoError(t, err)
			if len(test.imports) == 1 {
				require.Zero(t, resolved, "a target no import evaluated has no state to compare")
			}
		})
	}
}
