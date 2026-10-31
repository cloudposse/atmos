package configuration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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

func TestPrepareTranslatesErrorsExclusionAndFeatures(t *testing.T) {
	session, provider, info := preparationSession(t)
	signal, err := adapter.EncodeObject(map[string]any{"flag": true})
	require.NoError(t, err)
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{
		Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{Backend: signal},
		Errors: &pluginv1.ComponentErrorsConfiguration{
			Retry:  []*pluginv1.ComponentRetryPolicy{{Name: "net", RetryableErrors: []string{"timeout", "!fatal"}, MaxAttempts: 3, SleepIntervalSeconds: 4}},
			Ignore: []*pluginv1.ComponentIgnorePolicy{{Name: "known", IgnorableErrors: []string{"exists"}, Message: "fine", Signals: map[string]*pluginv1.ComponentValue{"alert": signal}}},
		},
		Exclude:      &pluginv1.ComponentExclude{Condition: true, Actions: []string{"plan"}},
		FeatureFlags: map[string]string{"flag": "true"},
		Diagnostics:  []*pluginv1.Diagnostic{{Detail: &pluginv1.ErrorDetail{Code: "read_config_before_hook_ignored", Summary: "never runs", Severity: pluginv1.Severity_SEVERITY_WARNING}}},
	}, nil)
	prepared, err := session.Prepare(t.Context(), info)
	require.NoError(t, err)

	require.NotNil(t, prepared.Execution, "error handling alone still produces execution metadata")
	require.Equal(t, []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout", "!fatal"}, MaxAttempts: 3, SleepInterval: 4 * time.Second}}, prepared.Execution.Errors.Retry)
	require.Equal(t, "known", prepared.Execution.Errors.Ignore[0].Name)
	require.Equal(t, "fine", prepared.Execution.Errors.Ignore[0].Message)
	require.Equal(t, map[string]any{"flag": true}, prepared.Execution.Errors.Ignore[0].Signals["alert"])
	require.Equal(t, &schema.ComponentExclusion{Actions: []string{"plan"}}, prepared.Exclusion)
	require.Equal(t, map[string]string{"flag": "true"}, prepared.FeatureFlags)
}

func TestPreparedErrorsRejectsPoliciesTheHostCannotHonor(t *testing.T) {
	empty, err := preparedErrors(&pluginv1.ComponentErrorsConfiguration{})
	require.NoError(t, err)
	require.Nil(t, empty)
	for name, source := range map[string]*pluginv1.ComponentErrorsConfiguration{
		"no attempts":         {Retry: []*pluginv1.ComponentRetryPolicy{{Name: "x", RetryableErrors: []string{"a"}}}},
		"nil retry":           {Retry: []*pluginv1.ComponentRetryPolicy{nil}},
		"bad retry pattern":   {Retry: []*pluginv1.ComponentRetryPolicy{{Name: "x", RetryableErrors: []string{"("}, MaxAttempts: 1}}},
		"bad ignore pattern":  {Ignore: []*pluginv1.ComponentIgnorePolicy{{Name: "x", IgnorableErrors: []string{"!("}}}},
		"nil ignore":          {Ignore: []*pluginv1.ComponentIgnorePolicy{nil}},
		"undecodable signals": {Ignore: []*pluginv1.ComponentIgnorePolicy{{Name: "x", Signals: map[string]*pluginv1.ComponentValue{"s": {TypeJson: []byte("nope")}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := preparedErrors(source)
			require.Error(t, err)
		})
	}
}

func TestPreparedExclusionNeedsItsCondition(t *testing.T) {
	require.Nil(t, preparedExclusion(nil))
	require.Nil(t, preparedExclusion(&pluginv1.ComponentExclude{Actions: []string{"plan"}}))
	require.Equal(t, &schema.ComponentExclusion{Actions: []string{"plan"}}, preparedExclusion(&pluginv1.ComponentExclude{Condition: true, Actions: []string{"plan"}}))
}

func TestTerraformBinaryMustMatchDiscovery(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "live"}
	configuration, err := adapter.EncodeObject(map[string]any{"command": "tofu"})
	require.NoError(t, err)
	discovered := &pluginv1.DiscoveredComponent{Configuration: configuration}

	require.NoError(t, checkTerraformBinary(info, discovered, ""), "a unit without terraform_binary has nothing to compare")
	require.NoError(t, checkTerraformBinary(info, discovered, "tofu"))
	err = checkTerraformBinary(info, discovered, "terraform")
	require.ErrorIs(t, err, errUtils.ErrComponentBinaryChanged)
	require.ErrorContains(t, err, `now selects "terraform"`)
	require.ErrorContains(t, err, `discovered with "tofu"`)
}

func TestPrepareRejectsAnExecutableChangeSinceDiscovery(t *testing.T) {
	session, provider, info := preparationSession(t)
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", TerraformBinary: "tofu", State: &pluginv1.ComponentStateIdentity{}}, nil)
	_, err := session.Prepare(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrComponentBinaryChanged)
}

// Compile-time sentinels for the schema fields the option tests depend on.
var (
	_ = schema.ComponentProviderOption{Value: "", Sensitive: true}
	_ = schema.AtmosConfiguration{ComponentProviderOptions: nil}
	_ = schema.ConfigAndStacksInfo{ComponentProviderOptions: nil}
)

func optionsConfig(t *testing.T, mode string) *schema.AtmosConfiguration {
	t.Helper()
	config := enabledConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Mode = mode
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	config.ComponentProviderOptions = map[string]map[string]schema.ComponentProviderOption{"terragrunt": {
		"quiet":    {Value: false},
		"retries":  {Value: 0},
		"feature":  {Value: map[string]any{"alpha": "true"}},
		"password": {Value: "s3cret", Sensitive: true},
	}}
	return config
}

func TestExplicitOptionsAndModeReachDiscoveryAndPreparation(t *testing.T) {
	for _, mode := range []string{"", "native", "executable"} {
		t.Run("mode "+mode, func(t *testing.T) {
			wantMode := mode
			if wantMode == "" {
				wantMode = "native"
			}
			check := func(operation *pluginv1.ComponentOperation) {
				require.Equal(t, wantMode, operation.Mode)
				require.Len(t, operation.Options, 4)
				quiet, err := adapter.DecodeValue(operation.Options["quiet"])
				require.NoError(t, err)
				require.Equal(t, false, quiet, "an explicit false is sent, not dropped")
				retries, err := adapter.DecodeValue(operation.Options["retries"])
				require.NoError(t, err)
				require.Equal(t, json.Number("0"), retries, "an explicit zero is sent, not dropped")
				require.Empty(t, operation.Options["quiet"].SensitivePaths)
				require.Len(t, operation.Options["password"].SensitivePaths, 1, "a sensitive option is marked at its root")
			}
			provider := NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
			provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
				check(request.Operation)
				return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
			})
			if wantMode != "executable" {
				provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
					check(request.Operation)
					return &pluginv1.PrepareComponentResponse{Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{}}, nil
				})
			}
			session, err := New(t.Context(), optionsConfig(t, mode), "plan", func(context.Context, []string) (map[string]Provider, error) {
				return map[string]Provider{"terragrunt": provider}, nil
			})
			require.NoError(t, err)
			info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "app", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}}
			if wantMode == "executable" {
				// The host never prepares an executable-mode unit; only discovery needs the plugin.
				_, err = session.Discover(t.Context())
				require.NoError(t, err)
				return
			}
			_, err = session.Prepare(t.Context(), info)
			require.NoError(t, err)
		})
	}
}

func TestOptionsAreAbsentWhenNoneWereSet(t *testing.T) {
	session, provider, info := preparationSession(t)
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		require.Empty(t, request.Operation.Options)
		require.Equal(t, "native", request.Operation.Mode)
		return &pluginv1.PrepareComponentResponse{Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{}}, nil
	})
	_, err := session.Prepare(t.Context(), info)
	require.NoError(t, err)
}

func TestPlanIdentityIncludesTheEffectiveOptions(t *testing.T) {
	identity := func(value string) string {
		config := enabledConfig(t)
		config.ComponentProviderOptions = map[string]map[string]schema.ComponentProviderOption{"terragrunt": {"level": {Value: value}}}
		mock := NewMockProvider(gomock.NewController(t))
		mock.EXPECT().ComponentType().Return("terraform").AnyTimes()
		mock.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil)
		provider := &identifiedProvider{Provider: mock, identity: func(context.Context) (string, error) { return "v1", nil }}
		session, err := New(t.Context(), config, "plan", func(context.Context, []string) (map[string]Provider, error) {
			return map[string]Provider{"terragrunt": provider}, nil
		})
		require.NoError(t, err)
		info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "app", SubCommand: "plan", ComponentSettingsSection: map[string]any{"configuration": true}}
		sum, err := session.PlanIdentity(t.Context(), info, nil)
		require.NoError(t, err)
		return sum
	}
	require.Equal(t, identity("debug"), identity("debug"))
	require.NotEqual(t, identity("debug"), identity("info"), "applying a plan reviewed with other options must be refused")
}

func TestPrepareRefusesAnExecutableModeUnit(t *testing.T) {
	session, _, info := preparationSession(t)
	info.ComponentSettingsSection = map[string]any{"configuration": map[string]any{"engine": "terragrunt"}}
	_, err := session.Prepare(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrExecutableModeUnsupported)
}

func TestExecutableDiscoveryEngineIsListed(t *testing.T) {
	instance := unit(t, "app")
	instance.Engine = "terragrunt"
	config, err := instanceConfig(instance, false)
	require.NoError(t, err)
	require.Equal(t, "terragrunt", config["settings"].(map[string]any)["configuration"].(map[string]any)["engine"])

	instance.Engine = "pulumi"
	_, err = instanceConfig(instance, false)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
}

func TestSettingsValidationAcceptsBothModes(t *testing.T) {
	for _, mode := range []string{"", "native", "executable"} {
		require.NoError(t, validateSettings("terragrunt", &schema.ComponentConfiguration{Mode: mode}), mode)
	}
	require.ErrorIs(t, validateSettings("terragrunt", &schema.ComponentConfiguration{Mode: "cli"}), errUtils.ErrPluginInvalidConfig)
}

// captureAtmosLogs sends the Atmos logger to a buffer at the given level for one test.
func captureAtmosLogs(t *testing.T, level charm.Level) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previous := log.GetLevel()
	log.SetOutput(&output)
	log.SetLevel(level)
	adapter.ResetShownMessages()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(previous)
		adapter.ResetShownMessages()
	})
	return &output
}

func TestPrepareAsksForTheAtmosLogLevelAndRendersEngineLogsScoped(t *testing.T) {
	session, provider, info := preparationSession(t)
	var sent *pluginv1.ComponentRequest
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		sent = request
		return &pluginv1.PrepareComponentResponse{
			Engine: "opentofu", State: &pluginv1.ComponentStateIdentity{BackendType: "local"},
			Logs: []*pluginv1.LogEntry{
				{Level: pluginv1.LogLevel_LOG_LEVEL_DEBUG, Message: "reading include", Fields: map[string]string{"file": "root.hcl"}},
				{Level: pluginv1.LogLevel_LOG_LEVEL_TRACE, Message: "very detailed"},
			},
		}, nil
	})
	output := captureAtmosLogs(t, charm.DebugLevel)

	_, err := session.Prepare(t.Context(), info)

	require.NoError(t, err)
	require.Equal(t, "debug", sent.Invocation.LogLevel, "the plugin is told the Atmos log level")
	require.Contains(t, output.String(), "reading include")
	require.Contains(t, output.String(), "component=app")
	require.Contains(t, output.String(), "stack=live")
	require.Contains(t, output.String(), "file=root.hcl")
	require.NotContains(t, output.String(), "very detailed", "the Atmos level still decides what is visible")
}

func TestPrepareNamesAnUnknownInput(t *testing.T) {
	session, provider, info := preparationSession(t)
	inputs := &pluginv1.ComponentValue{TypeJson: []byte(`["object",{"zone":"string"}]`), Msgpack: []byte{0x81, 0xa4, 'z', 'o', 'n', 'e', 0xd4, 0x00, 0x00}}
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).Return(&pluginv1.PrepareComponentResponse{Engine: "opentofu", Inputs: inputs, State: &pluginv1.ComponentStateIdentity{}}, nil)

	_, err := session.Prepare(t.Context(), info)

	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	require.Contains(t, err.Error()+strings.Join(errUtils.AllDetails(err), "\n"), "`zone`")
}

func TestDiscoveryAndStateRenderEngineLogs(t *testing.T) {
	// The request is built when the session starts, so the level must be set before it.
	output := captureAtmosLogs(t, charm.DebugLevel)
	config := enabledConfig(t)
	mock := NewMockProvider(gomock.NewController(t))
	mock.EXPECT().ComponentType().Return("terraform")
	var sent *pluginv1.DiscoverComponentsRequest
	mock.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		sent = request
		return &pluginv1.DiscoverComponentsResponse{
			Components: []*pluginv1.DiscoveredComponent{unit(t, "network")},
			Logs:       []*pluginv1.LogEntry{{Level: pluginv1.LogLevel_LOG_LEVEL_DEBUG, Message: "scanning live"}},
		}, nil
	})
	provider := &stateProviderFixture{Provider: mock, resolve: func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return &pluginv1.ResolveComponentStateResponse{
			State: &pluginv1.ComponentStateIdentity{BackendType: "local"},
			Logs:  []*pluginv1.LogEntry{{Level: pluginv1.LogLevel_LOG_LEVEL_DEBUG, Message: "resolving backend"}},
		}, nil
	}}
	session, err := New(t.Context(), config, "describe", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)

	_, err = session.Discover(t.Context())
	require.NoError(t, err)
	_, err = session.ResolveState(t.Context(), &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", SubCommand: "output", ComponentSettingsSection: map[string]any{"configuration": true}})
	require.NoError(t, err)

	require.Equal(t, "debug", sent.Invocation.LogLevel)
	require.Contains(t, output.String(), "scanning live")
	require.Contains(t, output.String(), "resolving backend")
	require.Contains(t, output.String(), "component=network")
}
