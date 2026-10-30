package configuration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

func executionResponse(t *testing.T, execution *pluginv1.ComponentExecutionConfiguration) *pluginv1.PrepareComponentResponse {
	t.Helper()
	backend, err := adapter.EncodeObject(map[string]any{})
	require.NoError(t, err)
	return &pluginv1.PrepareComponentResponse{
		Engine:    "opentofu",
		State:     &pluginv1.ComponentStateIdentity{BackendType: "s3", Backend: backend, Workspace: "default"},
		Execution: execution,
	}
}

// roundTrip sends the response over the wire format so the test covers field numbers and names.
func roundTrip(t *testing.T, response *pluginv1.PrepareComponentResponse) *pluginv1.PrepareComponentResponse {
	t.Helper()
	data, err := proto.Marshal(response)
	require.NoError(t, err)
	decoded := &pluginv1.PrepareComponentResponse{}
	require.NoError(t, proto.Unmarshal(data, decoded))
	return decoded
}

func TestPreparedConfigurationCarriesExecutionInDeclarationOrder(t *testing.T) {
	response := roundTrip(t, executionResponse(t, &pluginv1.ComponentExecutionConfiguration{
		ExtraArguments: []*pluginv1.ComponentExtraArguments{
			{Commands: []string{"plan"}, Arguments: []string{"-lock=false"}},
			{Commands: []string{"apply", "plan"}, RequiredVarFiles: []string{"r.tfvars"}, OptionalVarFiles: []string{"o.tfvars"}, Environment: map[string]string{"TF_LOG": "INFO"}},
		},
		BeforeHooks: []*pluginv1.ComponentHook{
			{Name: "z-first", Commands: []string{"plan"}, Execute: []string{"tool", "1"}},
			{Name: "a-second", Commands: []string{"plan"}, Execute: []string{"tool", "2"}, WorkingDirectory: "sub", SuppressStdout: true},
		},
		AfterHooks: []*pluginv1.ComponentHook{{Name: "after", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks: []*pluginv1.ComponentHook{{Name: "oops", Commands: []string{"apply"}, Execute: []string{"tool"}, OnErrors: []string{"a", "b"}}},
	}))
	got, err := preparedConfiguration(response)
	require.NoError(t, err)
	require.Equal(t, &schema.ComponentExecution{
		ExtraArguments: []schema.ComponentExtraArguments{
			{Commands: []string{"plan"}, Arguments: []string{"-lock=false"}},
			{Commands: []string{"apply", "plan"}, RequiredVarFiles: []string{"r.tfvars"}, OptionalVarFiles: []string{"o.tfvars"}, Environment: map[string]string{"TF_LOG": "INFO"}},
		},
		BeforeHooks: []schema.ComponentHook{
			{Name: "z-first", Commands: []string{"plan"}, Execute: []string{"tool", "1"}},
			{Name: "a-second", Commands: []string{"plan"}, Execute: []string{"tool", "2"}, WorkingDirectory: "sub", SuppressStdout: true},
		},
		AfterHooks: []schema.ComponentHook{{Name: "after", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks: []schema.ComponentHook{{Name: "oops", Commands: []string{"apply"}, Execute: []string{"tool"}, OnErrors: []string{"a", "b"}}},
	}, got.Execution)
}

func TestPreparedConfigurationOmitsEmptyExecution(t *testing.T) {
	for _, execution := range []*pluginv1.ComponentExecutionConfiguration{nil, {}} {
		got, err := preparedConfiguration(roundTrip(t, executionResponse(t, execution)))
		require.NoError(t, err)
		require.Nil(t, got.Execution)
	}
}

func TestPreparedConfigurationExecutionIsIsolatedFromResponse(t *testing.T) {
	response := executionResponse(t, &pluginv1.ComponentExecutionConfiguration{
		BeforeHooks: []*pluginv1.ComponentHook{{Name: "h", Commands: []string{"plan"}, Execute: []string{"tool", "a"}}},
	})
	got, err := preparedConfiguration(response)
	require.NoError(t, err)
	// Result to source.
	got.Execution.BeforeHooks[0].Execute[1] = "changed"
	require.Equal(t, []string{"tool", "a"}, response.Execution.BeforeHooks[0].Execute)
	// Source to result.
	response.Execution.BeforeHooks[0].Commands[0] = "apply"
	require.Equal(t, []string{"plan"}, got.Execution.BeforeHooks[0].Commands)
}

func TestPreparedConfigurationRejectsUnrunnableHooks(t *testing.T) {
	for _, test := range []struct {
		name      string
		execution *pluginv1.ComponentExecutionConfiguration
	}{
		{"no command", &pluginv1.ComponentExecutionConfiguration{BeforeHooks: []*pluginv1.ComponentHook{{Name: "h", Commands: []string{"plan"}}}}},
		{"bad pattern", &pluginv1.ComponentExecutionConfiguration{ErrorHooks: []*pluginv1.ComponentHook{{Name: "h", Commands: []string{"plan"}, Execute: []string{"t"}, OnErrors: []string{"("}}}}},
		{"nil hook", &pluginv1.ComponentExecutionConfiguration{AfterHooks: []*pluginv1.ComponentHook{nil}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := preparedConfiguration(executionResponse(t, test.execution))
			require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
		})
	}
}
