package configuration

import (
	"context"
	"errors"
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
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil).MaxTimes(1)
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
	}, prepared.Files)
	require.Equal(t, []schema.ComponentConfigurationSource{{Path: "root.hcl"}, {Path: "modules", Directory: true}}, prepared.Sources)

	require.Equal(t, []string{"-var=a=1", "-lock=false"}, sent.Operation.TerraformArguments)
	info.AdditionalArgsAndFlags[0] = "mutated"
	require.Equal(t, "-var=a=1", sent.Operation.TerraformArguments[0], "the request owns a copy of the arguments")
}

func TestEngineMismatchNamesTheCommandAndBothEngines(t *testing.T) {
	session, provider, info := preparationSession(t)
	info.Command = "terraform"
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "terraform", State: &pluginv1.ComponentStateIdentity{}}, nil)

	_, err := session.Prepare(t.Context(), info)

	require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
	require.ErrorContains(t, err, `command "terraform" selects the terraform engine`)
	require.ErrorContains(t, err, "discovered for opentofu")
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
		{"engine changed by a command override", func(p *MockProvider, _ *schema.ConfigAndStacksInfo) {
			p.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "terraform", State: &pluginv1.ComponentStateIdentity{}}, nil)
		}, errUtils.ErrConfigurationOverrideUnsupported},
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
		name   string
		source any
		want   bool
	}{
		{name: "no source in the section"},
		{name: "same string", source: "./modules/unit"},
		{name: "same uri", source: map[string]any{"uri": "./modules/unit", "version": "1.2.3"}},
		{name: "settings without a uri", source: map[string]any{"version": "1.2.3"}},
		{name: "another uri", source: map[string]any{"uri": "./modules/other"}, want: true},
		{name: "another string", source: "./modules/other", want: true},
		{name: "unsupported shape", source: 42},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, provider, info := preparationSession(t)
			info.ComponentSection = map[string]any{"source": test.source}
			if test.want {
				_, err := session.Prepare(t.Context(), info)
				require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
				require.ErrorContains(t, err, "./modules/unit")
				require.ErrorContains(t, err, "./modules/other")
				return
			}
			provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{}}, nil)
			_, err := session.Prepare(t.Context(), info)
			require.NoError(t, err)
		})
	}
}
