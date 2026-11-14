package configuration

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preparationSession discovers one "app" component and returns the mock that will prepare it.
func preparationSession(t *testing.T) (*Session, *MockProvider, *schema.ConfigAndStacksInfo) {
	t.Helper()
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	// The session discovers lazily, and Prepare rediscovers to revalidate dependencies.
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil).AnyTimes()
	session, err := New(t.Context(), enabledConfig(t), "apply", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "app", SubCommand: "apply", ComponentSettingsSection: map[string]any{"configuration": true}}
	return session, provider, info
}

func TestPrepareTranslatesGeneratedArtifactsAndWatchedSources(t *testing.T) {
	session, provider, info := preparationSession(t)
	info.AdditionalArgsAndFlags = []string{"-var=a=1", "-lock=false"}
	backend, err := adapter.EncodeObject(map[string]any{"bucket": "state"})
	require.NoError(t, err)
	var sent *pluginv1.ComponentRequest
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		sent = request
		return &pluginv1.PrepareComponentResponse{
			Engine: "opentofu", BackendGenerated: true,
			State: &pluginv1.ComponentStateIdentity{BackendType: "s3", Backend: backend, Workspace: "stage"},
			Files: []*pluginv1.ComponentGeneratedFile{
				{Path: "backend.tf.json", Content: []byte("{}"), IfExists: "overwrite", Sensitive: true},
				{Path: "provider.tf", Content: []byte("p"), IfExists: "skip"},
				{Path: ".terraform.lock.hcl", Content: []byte("pins"), IfExists: "overwrite", UnitFileCopy: true},
			},
			Sources: []*pluginv1.ComponentSource{
				{Path: "root.hcl", Kind: pluginv1.ComponentSource_FILE},
				{Path: "modules", Kind: pluginv1.ComponentSource_DIRECTORY},
			},
		}, nil
	})
	prepared, err := session.Prepare(t.Context(), info)
	require.NoError(t, err)

	require.True(t, prepared.BackendGenerated)
	require.Equal(t, []schema.ComponentConfigurationFile{
		{Path: "backend.tf.json", Content: []byte("{}"), IfExists: "overwrite", Sensitive: true},
		{Path: "provider.tf", Content: []byte("p"), IfExists: "skip"},
		{Path: ".terraform.lock.hcl", Content: []byte("pins"), IfExists: "overwrite", UnitFileCopy: true},
	}, prepared.Files)
	require.Equal(t, []schema.ComponentConfigurationSource{{Path: "root.hcl"}, {Path: "modules", Directory: true}}, prepared.Sources)

	require.Equal(t, []string{"-var=a=1", "-lock=false"}, sent.Operation.TerraformArguments)
	info.AdditionalArgsAndFlags[0] = "mutated"
	require.Equal(t, "-var=a=1", sent.Operation.TerraformArguments[0], "the request owns a copy of the arguments")
}

func TestEngineMismatchNamesTheCommandAndBothEngines(t *testing.T) {
	session, _, info := preparationSession(t)
	info.Command = "terraform"

	_, err := session.Prepare(t.Context(), info)

	require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
	require.ErrorContains(t, err, `command "terraform" selects the terraform engine`)
	require.ErrorContains(t, err, "discovered for opentofu")
}

func TestPrepareRejectsDependenciesChangedSinceDiscovery(t *testing.T) {
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	network, app := unit(t, "network"), unit(t, "app")
	moved := unit(t, "app")
	moved.Dependencies = []*pluginv1.ComponentDependency{{Target: network.Address, Kind: pluginv1.ComponentDependency_OUTPUT}}
	gomock.InOrder(
		provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network, app}}, nil),
		provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{network, moved}}, nil),
	)
	session, err := New(t.Context(), enabledConfig(t), "apply", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "app", SubCommand: "apply", ComponentSettingsSection: map[string]any{"configuration": true}}

	prepared, err := session.Prepare(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorContains(t, err, "preparation changed discovered dependency paths")
	require.Nil(t, prepared, "Prepare must not run when dependencies changed")
}

func TestPrepareFailsClosed(t *testing.T) {
	failure := errors.New("provider crashed")
	for _, test := range []struct {
		name  string
		setup func(*MockProvider, *schema.ConfigAndStacksInfo)
		want  error
	}{
		{"undiscovered component", func(_ *MockProvider, info *schema.ConfigAndStacksInfo) { info.ComponentFromArg = "absent" }, errUtils.ErrPluginInvalidConfig},
		{"provider failure", func(p *MockProvider, _ *schema.ConfigAndStacksInfo) {
			p.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(nil, failure)
		}, failure},
		{"provider returns an unexpected engine", func(p *MockProvider, _ *schema.ConfigAndStacksInfo) {
			p.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "terraform", State: &pluginv1.ComponentStateIdentity{}}, nil)
		}, errUtils.ErrPluginProtocol},
		{"saved plan", func(_ *MockProvider, info *schema.ConfigAndStacksInfo) { info.UseTerraformPlan = true }, errUtils.ErrPluginUnsupportedExtension},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, provider, info := preparationSession(t)
			test.setup(provider, info)
			prepared, err := session.Prepare(t.Context(), info)
			require.ErrorIs(t, err, test.want)
			require.Nil(t, prepared)
		})
	}
}

func TestPrepareAcceptsTheEngineOfTheSelectedCommand(t *testing.T) {
	for _, test := range []struct {
		name, command, engine string
		want                  error
	}{
		{"command override selects OpenTofu", "/opt/bin/tofu", "opentofu", nil},
		{"versioned OpenTofu binary", "/opt/bin/TOFU-1.8", "opentofu", nil},
		{"command override selects Terraform", "/opt/bin/terraform", "terraform", nil},
		{"provider disagrees with the command", "/opt/bin/terraform", "opentofu", errUtils.ErrPluginProtocol},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, provider, info := preparationSession(t)
			info.Command, info.ComponentCommand = test.command, test.command
			provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
				require.Equal(t, test.command, request.Invocation.Environment["ATMOS_TERRAFORM_BINARY"])
				return &pluginv1.PrepareComponentResponse{Engine: test.engine, State: &pluginv1.ComponentStateIdentity{}}, nil
			})
			_, err := session.Prepare(t.Context(), info)
			if test.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestPreparedExecutionRejectsNilEntriesInEveryList(t *testing.T) {
	for _, test := range []struct {
		name      string
		execution *pluginv1.ComponentExecutionConfiguration
	}{
		{"extra arguments", &pluginv1.ComponentExecutionConfiguration{ExtraArguments: []*pluginv1.ComponentExtraArguments{nil}}},
		{"before hooks", &pluginv1.ComponentExecutionConfiguration{BeforeHooks: []*pluginv1.ComponentHook{nil}}},
		{"after hooks", &pluginv1.ComponentExecutionConfiguration{AfterHooks: []*pluginv1.ComponentHook{nil}}},
		{"error hooks", &pluginv1.ComponentExecutionConfiguration{ErrorHooks: []*pluginv1.ComponentHook{nil}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			execution, err := preparedExecution(test.execution)
			require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
			require.Nil(t, execution)
		})
	}
}

func TestPrepareRejectsAnotherModuleSourceButAllowsOtherSourceSettings(t *testing.T) {
	for _, test := range []struct {
		name     string
		source   any
		want     bool
		redacted []string
	}{
		{name: "no source in the section"},
		{name: "same string", source: "./modules/unit"},
		{name: "same uri", source: map[string]any{"uri": "./modules/unit", "version": "1.2.3"}},
		{name: "settings without a uri", source: map[string]any{"version": "1.2.3"}},
		{name: "another uri", source: map[string]any{"uri": "./modules/other"}, want: true},
		{name: "another string", source: "./modules/other", want: true},
		{name: "unsupported shape", source: 42},
		{name: "credentials in another uri", source: "git::https://user:token-4f2a@example.com/org/modules.git?ref=v1&sshkey=key-9c1e", want: true, redacted: []string{"token-4f2a", "key-9c1e"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, provider, info := preparationSession(t)
			info.ComponentSection = map[string]any{"source": test.source}
			if test.want {
				_, err := session.Prepare(t.Context(), info)
				require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
				require.ErrorContains(t, err, "./modules/unit")
				for _, secret := range test.redacted {
					require.NotContains(t, err.Error(), secret)
				}
				if test.redacted == nil {
					require.ErrorContains(t, err, "./modules/other")
				}
				return
			}
			provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{}}, nil)
			_, err := session.Prepare(t.Context(), info)
			require.NoError(t, err)
		})
	}
}

func TestSessionResolvesStateWithoutPreparation(t *testing.T) {
	config := enabledConfig(t)
	mock := NewMockProvider(gomock.NewController(t))
	mock.EXPECT().ComponentType().Return("terraform")
	mock.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "network")}}, nil)
	backend, err := adapter.EncodeObject(map[string]any{"path": "state.tfstate"})
	require.NoError(t, err)
	mock.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		require.Nil(t, req.Effects)
		require.Equal(t, "output", req.Operation.Command)
		require.Equal(t, "selected", req.Invocation.Environment["IDENTITY"])
		require.Equal(t, "stage", req.Invocation.Environment["TF_WORKSPACE"])
		return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "stage", WorkingDirectory: config.BasePathAbsolute}}, nil
	})
	session, err := New(t.Context(), config, "describe", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": mock}, nil
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", SubCommand: "output", TerraformWorkspace: "stage", ComponentSettingsSection: map[string]any{"configuration": true}, SanitizedEnv: []string{"IDENTITY=selected"}}
	state, err := session.ResolveState(t.Context(), info)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(config.BasePathAbsolute, "state.tfstate"), state.Backend["path"])
	require.Equal(t, "stage", state.Workspace)
	mock.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return nil, context.Canceled
	})
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, context.Canceled)
	mock.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return &pluginv1.ResolveComponentStateResponse{}, nil
	})
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	mock.EXPECT().ResolveState(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{Backend: &pluginv1.ComponentValue{}}}, nil
	})
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	info.ComponentFromArg = "absent"
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	info.ComponentSettingsSection = nil
	state, err = session.ResolveState(t.Context(), info)
	require.NoError(t, err)
	require.Nil(t, state)
}
