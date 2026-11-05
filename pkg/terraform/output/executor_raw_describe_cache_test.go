package output

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// rawDescribeCall invokes one public lookup entry point with SkipInit set as requested
// and no auth manager, so SkipInit describes the target without YAML functions.
type rawDescribeCall func(executor *Executor, config *schema.AtmosConfiguration, skipInit bool) (any, error)

func rawDescribeCalls() map[string]rawDescribeCall {
	return map[string]rawDescribeCall{
		"GetOutputWithOptions": func(executor *Executor, config *schema.AtmosConfiguration, skipInit bool) (any, error) {
			value, _, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, &OutputOptions{SkipInit: skipInit})
			return value, err
		},
		"GetOutputWithOptions/inspection": func(executor *Executor, config *schema.AtmosConfiguration, skipInit bool) (any, error) {
			value, _, err := executor.GetOutputWithOptions(config, "stack", "component", "value", false, nil, nil, &OutputOptions{SkipInit: skipInit, SecretsMaskOnly: true})
			return value, err
		},
		"GetAllOutputs": func(executor *Executor, config *schema.AtmosConfiguration, skipInit bool) (any, error) {
			values, err := executor.GetAllOutputs(config, "component", "stack", skipInit, nil, nil)
			return values["value"], err
		},
	}
}

// Static outputs from a raw describe (YAML functions unprocessed) must not satisfy a later
// lookup that expects fully processed values.
func TestExecutor_RawDescribeDoesNotShareProcessedCacheEntry(t *testing.T) {
	for name, call := range rawDescribeCalls() {
		t.Run(name, func(t *testing.T) {
			ResetOutputsCache()
			t.Cleanup(ResetOutputsCache)
			ctrl := gomock.NewController(t)
			describer := NewMockComponentDescriber(ctrl)
			getter := NewMockStaticRemoteStateGetter(ctrl)
			executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
			config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
			var processed []bool
			describer.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(func(params *DescribeComponentParams) (map[string]any, error) {
				processed = append(processed, params.ProcessYamlFunctions)
				return map[string]any{"processed": params.ProcessYamlFunctions}, nil
			}).Times(2)
			getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).DoAndReturn(func(sections *map[string]any) map[string]any {
				return map[string]any{"value": map[bool]string{true: "processed", false: "raw"}[(*sections)["processed"].(bool)]}
			}).Times(2)

			for _, skipInit := range []bool{true, false, true, false} {
				value, err := call(executor, config, skipInit)
				require.NoError(t, err)
				require.Equal(t, map[bool]string{true: "raw", false: "processed"}[skipInit], value)
			}
			require.Equal(t, []bool{false, true}, processed, "each describe mode is fetched once and then reused")
		})
	}
}

// Repeating a raw lookup reuses its own entry.
func TestExecutor_RawDescribeReusesRawCacheEntry(t *testing.T) {
	for name, call := range rawDescribeCalls() {
		t.Run(name, func(t *testing.T) {
			ResetOutputsCache()
			t.Cleanup(ResetOutputsCache)
			ctrl := gomock.NewController(t)
			describer := NewMockComponentDescriber(ctrl)
			getter := NewMockStaticRemoteStateGetter(ctrl)
			executor := NewExecutor(describer, WithStaticRemoteStateGetter(getter))
			config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "debug"}}
			describer.EXPECT().DescribeComponent(gomock.Any()).Return(map[string]any{}, nil).Times(1)
			getter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(map[string]any{"value": "raw"}).Times(1)

			for range 3 {
				value, err := call(executor, config, true)
				require.NoError(t, err)
				require.Equal(t, "raw", value)
			}
		})
	}
}

func TestProcessYamlFunctions(t *testing.T) {
	manager := any(&struct{ name string }{"manager"})
	tests := []struct {
		name    string
		opts    *OutputOptions
		manager any
		want    bool
	}{
		{name: "nil options", want: true},
		{name: "defaults", opts: &OutputOptions{}, want: true},
		{name: "skip init without manager", opts: &OutputOptions{SkipInit: true}, want: false},
		{name: "skip init with manager", opts: &OutputOptions{SkipInit: true}, manager: manager, want: true},
		{name: "manager without skip init", opts: &OutputOptions{}, manager: manager, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, processYamlFunctions(tt.opts, tt.manager))
		})
	}
}
