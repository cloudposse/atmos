package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

func enabledConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	return &schema.AtmosConfiguration{BasePathAbsolute: t.TempDir(), Components: schema.Components{Terraform: schema.Terraform{Command: "tofu", ConfigurationProviders: map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true, Discovery: schema.ComponentDiscoveryConfig{Enabled: true, IncludedPaths: []string{"live/**"}}}}}}}
}

func unit(t *testing.T, name string) *pluginv1.DiscoveredComponent {
	t.Helper()
	value, err := adapter.EncodeObject(map[string]any{"source": "./modules/unit", "vars": map[string]any{}})
	require.NoError(t, err)
	return &pluginv1.DiscoveredComponent{Address: &pluginv1.ComponentAddress{Stack: "live", Component: name, ComponentType: "terraform"}, Provider: "terragrunt", Engine: "opentofu", Source: "live/" + name + "/terragrunt.hcl", Configuration: value, Sources: []*pluginv1.ComponentSource{{Path: "root.hcl", Kind: pluginv1.ComponentSource_FILE}, {Path: "modules", Kind: pluginv1.ComponentSource_DIRECTORY}}}
}

func TestSessionDiscoveryAndPreparation(t *testing.T) {
	config := enabledConfig(t)
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform")
	network, app := unit(t, "network"), unit(t, "app")
	app.Dependencies = []*pluginv1.ComponentDependency{{Target: network.Address, Kind: pluginv1.ComponentDependency_OUTPUT}}
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Equal(t, []string{"live/**"}, request.IncludePaths)
		require.Nil(t, request.Effects)
		require.Equal(t, "tofu", request.Invocation.Environment["ATMOS_TERRAFORM_BINARY"])
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network, app}}, nil
	})
	calls := 0
	session, err := New(t.Context(), config, "apply", func(_ context.Context, names []string) (map[string]Provider, error) {
		calls++
		require.Equal(t, []string{"terragrunt"}, names)
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	require.Zero(t, calls)
	// Changing configuration after construction must not alter this invocation.
	config.Components.Terraform.ConfigurationProviders["terragrunt"].Discovery.IncludedPaths[0] = "different/**"
	first, err := session.Discover(t.Context())
	require.NoError(t, err)
	components := first["live"]["components"].(map[string]any)["terraform"].(map[string]any)
	appConfig := components["app"].(map[string]any)
	require.Equal(t, []any{"root.hcl"}, appConfig["dependencies"].(map[string]any)["files"])
	require.Equal(t, []any{map[string]any{"name": "network", "stack": "live", "kind": "terraform"}}, appConfig["dependencies"].(map[string]any)["components"])
	components["app"] = "corrupt"
	second, err := session.Discover(t.Context())
	require.NoError(t, err)
	require.IsType(t, map[string]any{}, second["live"]["components"].(map[string]any)["terraform"].(map[string]any)["app"])
	require.Equal(t, 1, calls)
	inputs, err := adapter.EncodeObject(map[string]any{"large": json.Number("9007199254740993")})
	require.NoError(t, err)
	backend, err := adapter.EncodeObject(map[string]any{"path": "state.tfstate"})
	require.NoError(t, err)
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		require.Equal(t, "apply", request.Operation.Command)
		require.True(t, request.Effects.StateReads)
		require.Equal(t, "stage", request.Invocation.Environment["TF_WORKSPACE"])
		require.Equal(t, "override", request.Invocation.Environment["FROM_COMPONENT"])
		return &pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", Inputs: inputs, State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "stage", WorkingDirectory: config.BasePathAbsolute}, Files: []*pluginv1.ComponentGeneratedFile{{Path: "context.tf", Content: []byte("{{ literal }}"), IfExists: "overwrite"}}}, nil
	})
	prepared, err := session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "app", Command: "tofu", SubCommand: "apply", TerraformWorkspace: "stage", ComponentSettingsSection: map[string]any{"configuration": true}, ComponentEnvList: []string{"FROM_COMPONENT=original", "FROM_COMPONENT=override"}, ComponentEnvSection: map[string]any{"FROM_COMPONENT": "override"}})
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), prepared.Inputs["large"])
	require.Equal(t, filepath.Join(config.BasePathAbsolute, "state.tfstate"), prepared.Backend["path"])
	require.Equal(t, "{{ literal }}", string(prepared.Files[0].Content))
}

func TestSessionRejectsInvalidScope(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*schema.ComponentConfiguration)
		want   error
	}{
		{"mode", func(c *schema.ComponentConfiguration) { c.Mode = "cli" }, errUtils.ErrPluginInvalidConfig},
		{"escape", func(c *schema.ComponentConfiguration) { c.Discovery.IncludedPaths = []string{"../**"} }, errUtils.ErrPluginInvalidConfig},
		{"options", func(c *schema.ComponentConfiguration) { c.Options = map[string]any{"enabled": false} }, errUtils.ErrPluginInvalidConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := enabledConfig(t)
			settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
			test.change(&settings)
			config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
			// Reading configuration never fails on plugin settings, so `atmos config set` can repair them.
			session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
				t.Fatal("must not load")
				return nil, nil
			})
			require.NoError(t, err)
			_, err = session.Discover(t.Context())
			require.ErrorIs(t, err, test.want)
			require.NotEmpty(t, errUtils.AllHints(err), "the error must say how to fix the setting")
		})
	}
	config := enabledConfig(t)
	_, err := New(t.Context(), config, "list", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Provider = "terragrunt"
	config.Components.Terraform.ConfigurationProviders["duplicate"] = settings
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) { return nil, nil })
	require.NoError(t, err)
	_, err = session.Discover(t.Context())
	require.ErrorIs(t, err, errUtils.ErrDuplicatePluginExtension)
}

// Enabling discovery without included_paths scopes it to the whole project instead of failing.
func TestDiscoveryWithoutIncludedPathsDefaultsToTheProject(t *testing.T) {
	config := enabledConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Discovery.IncludedPaths = nil
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform")
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Equal(t, []string{"**"}, request.IncludePaths)
		return &pluginv1.DiscoverComponentsResponse{}, nil
	})
	session, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	_, err = session.Discover(t.Context())
	require.NoError(t, err)
}

func TestSessionCancellationAndFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	session, err := New(ctx, enabledConfig(t), "list", func(ctx context.Context, _ []string) (map[string]Provider, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	})
	require.NoError(t, err)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := session.Discover(context.Background())
		require.ErrorIs(t, err, context.Canceled)
	}()
	<-started
	cancel()
	wg.Wait()
	<-finished
	_, err = New(ctx, enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) { return nil, nil })
	require.ErrorIs(t, err, context.Canceled)
	session, err = New(t.Context(), enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
		return nil, errUtils.ErrPluginInvalidConfig
	})
	require.NoError(t, err)
	_, err = session.Discover(t.Context())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	prepared, err := session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	require.Nil(t, prepared)
	_, err = session.Prepare(t.Context(), &schema.ConfigAndStacksInfo{UseTerraformPlan: true, ComponentSettingsSection: map[string]any{"configuration": true}})
	require.True(t, errors.Is(err, errUtils.ErrPluginUnsupportedExtension))
}

func TestSessionRejectsConflictingDiscovery(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(*pluginv1.DiscoveredComponent)
		duplicate bool
		want      error
	}{
		{name: "unsafe stack", change: func(c *pluginv1.DiscoveredComponent) { c.Address.Stack = "../outside" }, want: errUtils.ErrPluginProtocol},
		{name: "unsupported engine", change: func(c *pluginv1.DiscoveredComponent) { c.Engine = "terragrunt" }, want: errUtils.ErrPluginUnsupportedExtension},
		{name: "invalid values", change: func(c *pluginv1.DiscoveredComponent) { c.Configuration = &pluginv1.ComponentValue{} }, want: errUtils.ErrPluginProtocol},
		{name: "duplicate", duplicate: true, want: errUtils.ErrDuplicatePluginExtension},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ComponentType().Return("terraform")
			instance := unit(t, "app")
			if test.change != nil {
				test.change(instance)
			}
			components := []*pluginv1.DiscoveredComponent{instance}
			if test.duplicate {
				components = append(components, instance)
			}
			provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: components}, nil)
			session, err := New(t.Context(), enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) {
				return map[string]Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			result, err := session.Discover(t.Context())
			require.ErrorIs(t, err, test.want)
			require.Nil(t, result)
		})
	}
	session, err := New(t.Context(), enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) { return map[string]Provider{}, nil })
	require.NoError(t, err)
	_, err = session.Discover(t.Context())
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
}

func TestPreparedConfigurationRejectsUnsupportedValues(t *testing.T) {
	backend, err := adapter.EncodeObject(map[string]any{})
	require.NoError(t, err)
	response := &pluginv1.PrepareComponentResponse{Engine: "terraform", State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, WorkingDirectory: t.TempDir(), Workspace: "default"}}
	got, err := preparedConfiguration(response)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(response.State.WorkingDirectory, "terraform.tfstate"), got.Backend["path"])
	response.Files = []*pluginv1.ComponentGeneratedFile{{Path: "generated.tf", IfExists: "overwrite_terragrunt"}}
	got, err = preparedConfiguration(response)
	require.NoError(t, err)
	require.Equal(t, "overwrite_terragrunt", got.Files[0].IfExists)
	response.Files = nil
	response.Inputs = &pluginv1.ComponentValue{}
	_, err = preparedConfiguration(response)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	response.Inputs = nil
	response.State.Backend = &pluginv1.ComponentValue{}
	_, err = preparedConfiguration(response)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
}
