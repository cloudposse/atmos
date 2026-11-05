package output

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.AtmosConfiguration{BasePath: "", CliConfigPath: ""}

func TestExecutor_InspectionCachePartitions(t *testing.T) {
	for _, inspectFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "inspect-first", false: "execute-first"}[inspectFirst], func(t *testing.T) {
			ResetOutputsCache()
			t.Cleanup(ResetOutputsCache)
			ctrl := gomock.NewController(t)
			describer := NewMockComponentDescriber(ctrl)
			getter := NewMockStaticRemoteStateGetter(ctrl)
			executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
			config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
			describer.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(func(params *DescribeComponentParams) (map[string]any, error) {
				value := "resolved"
				if params.SecretsMaskOnly {
					value = "masked"
				}
				return map[string]any{"value": value}, nil
			}).Times(2)
			getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).DoAndReturn(func(sections *map[string]any) map[string]any { return *sections }).Times(2)
			for _, inspect := range []bool{inspectFirst, !inspectFirst, inspectFirst, !inspectFirst} {
				value, exists, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, &OutputOptions{SecretsMaskOnly: inspect})
				require.NoError(t, err)
				require.True(t, exists)
				expected := "resolved"
				if inspect {
					expected = "masked"
				}
				require.Equal(t, expected, value)
			}
		})
	}
}

func TestExecutor_OutputCacheConfigurationScope(t *testing.T) {
	for _, method := range []string{"output", "options", "all"} {
		for _, changed := range []string{"base", "config", "invocation"} {
			t.Run(method+"/"+changed, func(t *testing.T) {
				ResetOutputsCache()
				t.Cleanup(ResetOutputsCache)
				ctrl := gomock.NewController(t)
				describer := NewMockComponentDescriber(ctrl)
				getter := NewMockStaticRemoteStateGetter(ctrl)
				executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
				root := t.TempDir()
				head := &schema.AtmosConfiguration{BasePath: root, CliConfigPath: root, Logs: schema.Logs{Level: "debug"}}
				base := *head
				switch changed {
				case "base":
					base.BasePath = filepath.Join(root, "base")
				case "config":
					base.CliConfigPath = filepath.Join(root, "base")
				case "invocation":
					head.AuthManager = authtypes.NewMockAuthManager(ctrl)
					base.AuthManager = authtypes.NewMockAuthManager(ctrl)
				}
				describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil).Times(2)
				gomock.InOrder(
					getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "head"}),
					getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "base"}),
				)
				for _, index := range []int{0, 1, 0, 1} {
					config := []*schema.AtmosConfiguration{head, &base}[index]
					var value any
					var err error
					switch method {
					case "all":
						var values map[string]any
						values, err = executor.GetAllOutputs(config, "component", "stack", false, nil, nil)
						value = values["value"]
					case "options":
						value, _, err = executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, &OutputOptions{})
					default:
						value, _, err = executor.GetOutput(config, "stack", "component", "value", false, nil, nil)
					}
					require.NoError(t, err)
					require.Equal(t, []string{"head", "base"}[index], value)
				}
			})
		}
	}
}

func TestExecutor_InspectionCacheRetriesDescribeErrors(t *testing.T) {
	ResetOutputsCache()
	t.Cleanup(ResetOutputsCache)
	ctrl := gomock.NewController(t)
	describer := NewMockComponentDescriber(ctrl)
	getter := NewMockStaticRemoteStateGetter(ctrl)
	executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
	config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
	gomock.InOrder(
		describer.EXPECT().DescribeComponent(gomock.Any()).Return(nil, errUtils.ErrTerraformOutputFailed),
		describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil),
	)
	getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "recovered"})
	opts := &OutputOptions{SecretsMaskOnly: true}
	_, _, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, opts)
	require.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
	for range 2 {
		value, exists, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, opts)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, "recovered", value)
	}
}

func TestExecutor_InvalidateAuthenticatedInspectionOutputs(t *testing.T) {
	ResetOutputsCache()
	t.Cleanup(ResetOutputsCache)
	ctrl := gomock.NewController(t)
	describer := NewMockComponentDescriber(ctrl)
	getter := NewMockStaticRemoteStateGetter(ctrl)
	manager := authtypes.NewMockAuthManager(ctrl)
	manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{Identity: "identity"}).AnyTimes()
	executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
	config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
	describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil).Times(8)
	gomock.InOrder(
		getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "old"}).Times(4),
		getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "new"}).Times(4),
	)
	for _, expected := range []string{"old", "new"} {
		for _, scope := range []string{"head", "base"} {
			config.BasePath = scope
			for _, inspect := range []bool{false, true} {
				value, _, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, manager, &OutputOptions{SecretsMaskOnly: inspect})
				require.NoError(t, err)
				require.Equal(t, expected, value)
			}
		}
		InvalidateComponentOutputs("stack", "component")
	}
}

func TestExecutor_InspectionCacheDisabledAuthNonStatic(t *testing.T) {
	ResetOutputsCache()
	t.Cleanup(ResetOutputsCache)
	ctrl := gomock.NewController(t)
	describer := NewMockComponentDescriber(ctrl)
	runner := NewMockTerraformRunner(ctrl)
	backend := NewMockBackendGenerator(ctrl)
	executor := NewExecutor(describer, WithRunnerFactory(func(_, _ string) (TerraformRunner, error) { return runner, nil }), WithBackendGenerator(backend))
	config := validAtmosConfig(t)
	config.Logs.Level = "debug"
	managers := []*authtypes.MockAuthManager{authtypes.NewMockAuthManager(ctrl), authtypes.NewMockAuthManager(ctrl)}
	for _, manager := range managers {
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthDisabled: true}).AnyTimes()
	}
	// Separate disabled managers retain isolation; a stable target manager reuses its fetch.
	describer.EXPECT().DescribeComponent(gomock.Any()).Return(validSections(), nil).Times(3)
	backend.EXPECT().GenerateBackendIfNeeded(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(3)
	backend.EXPECT().GenerateProvidersIfNeeded(gomock.Any(), gomock.Any()).Return(nil).Times(3)
	runner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	gomock.InOrder(
		// Windows retries a failed Output; the failure must persist across that budget.
		runner.EXPECT().Output(gomock.Any()).Return(nil, errUtils.ErrTerraformOutputFailed).Times(outputCallsBeforeGivingUp()),
		runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"value": {Value: []byte(`"first"`)}}, nil),
		runner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{"value": {Value: []byte(`"second"`)}}, nil),
	)
	opts := &OutputOptions{SecretsMaskOnly: true, SkipInit: true}
	_, _, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, managers[0], opts)
	require.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
	for _, index := range []int{0, 0, 1, 1, 0} {
		value, exists, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, managers[index], opts)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, []string{"first", "second"}[index], value)
	}
}

func TestExecutor_InspectionCacheRetriesComputedOutputs(t *testing.T) {
	ResetOutputsCache()
	t.Cleanup(ResetOutputsCache)
	ctrl := gomock.NewController(t)
	describer := NewMockComponentDescriber(ctrl)
	getter := NewMockStaticRemoteStateGetter(ctrl)
	executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
	config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
	describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil).Times(2)
	gomock.InOrder(
		getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": degradation.AtmosComputedValue{}}),
		getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "ready"}),
	)
	opts := &OutputOptions{SecretsMaskOnly: true}
	for _, expected := range []string{"(computed)", "ready", "ready"} {
		value, exists, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, opts)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, expected, value)
	}
}
