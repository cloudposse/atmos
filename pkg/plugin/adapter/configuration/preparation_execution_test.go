package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
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
		{"unrunnable apply hook", &pluginv1.ComponentExecutionConfiguration{Apply: &pluginv1.ComponentExecutionConfiguration{AfterHooks: []*pluginv1.ComponentHook{{Name: "h", Commands: []string{"apply"}}}}}},
		{"nested apply", &pluginv1.ComponentExecutionConfiguration{Apply: &pluginv1.ComponentExecutionConfiguration{Apply: &pluginv1.ComponentExecutionConfiguration{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := preparedConfiguration(executionResponse(t, test.execution))
			require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
		})
	}
}

// sensitiveString encodes a string the way a provider sends a secret: the whole value is sensitive.
func sensitiveString(t *testing.T, text string) *pluginv1.ComponentValue {
	t.Helper()
	typeJSON, err := ctyjson.MarshalType(cty.String)
	require.NoError(t, err)
	payload, err := msgpack.Marshal(cty.StringVal(text), cty.String)
	require.NoError(t, err)
	return &pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload, SensitivePaths: []*pluginv1.ComponentValuePath{{}}}
}

func TestPreparedConfigurationCarriesIAM(t *testing.T) {
	response := executionResponse(t, nil)
	response.Iam = &pluginv1.ComponentIam{Role: "arn:aws:iam::1:role/x", SessionName: "s", DurationSeconds: 900}
	got, err := preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.Equal(t, &schema.ComponentIAM{Role: "arn:aws:iam::1:role/x", SessionName: "s", DurationSeconds: 900}, got.IAM)

	response.Iam = nil
	got, err = preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.Nil(t, got.IAM)

	response.Iam = &pluginv1.ComponentIam{}
	got, err = preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.Nil(t, got.IAM, "an IAM message without a role asks for nothing")
}

func TestPreparedRegistryAuthorizationRequiresSameOriginAndSensitivity(t *testing.T) {
	response := executionResponse(t, nil)
	response.ModuleSource = "https://registry.example/module.tar.gz//subdir"
	response.ModuleSourceAuthOrigin = "https://registry.example"
	response.ModuleSourceAuthorization = sensitiveString(t, "Bearer registry-secret")
	prepared, err := preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.Equal(t, &schema.SourceAuthorization{Origin: "https://registry.example", Header: "Bearer registry-secret"}, prepared.SourceAuthorization)

	response.ModuleSourceAuthOrigin = "https://other.example"
	_, err = preparedConfiguration(roundTrip(t, response))
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	response.ModuleSourceAuthOrigin = "https://registry.example"
	response.ModuleSourceAuthorization.SensitivePaths = nil
	_, err = preparedConfiguration(roundTrip(t, response))
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
}

func TestPreparedIAMDistinguishesTokenFilesFromLiteralTokens(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(file, []byte("file-token\n"), 0o600))

	fromFile, err := preparedIAM(&pluginv1.ComponentIam{Role: "r", WebIdentityToken: sensitiveString(t, file)})
	require.NoError(t, err)
	require.Equal(t, &schema.ComponentIAM{Role: "r", WebIdentityTokenFile: file}, fromFile)

	literal, err := preparedIAM(&pluginv1.ComponentIam{Role: "r", WebIdentityToken: sensitiveString(t, "literal-token")})
	require.NoError(t, err)
	require.Equal(t, &schema.ComponentIAM{Role: "r", WebIdentityToken: "literal-token", WebIdentityTokenLiteral: true}, literal)

	// A literal token never reaches a serialized record.
	record, err := json.Marshal(literal)
	require.NoError(t, err)
	require.NotContains(t, string(record), "literal-token")

	// A directory is not a token file.
	directory, err := preparedIAM(&pluginv1.ComponentIam{Role: "r", WebIdentityToken: sensitiveString(t, t.TempDir())})
	require.NoError(t, err)
	require.Empty(t, directory.WebIdentityTokenFile)

	_, err = preparedIAM(&pluginv1.ComponentIam{Role: "r", WebIdentityToken: &pluginv1.ComponentValue{TypeJson: []byte("{")}})
	require.Error(t, err)
}

func TestDecodeStringRejectsNonStrings(t *testing.T) {
	text, err := adapter.DecodeString(nil)
	require.NoError(t, err)
	require.Empty(t, text)
	number, err := adapter.EncodeObject(map[string]any{"a": 1})
	require.NoError(t, err)
	_, err = adapter.DecodeString(number)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
}

// A plan preparation carries the apply-time evaluation separately. An empty one is kept,
// because it means apply runs nothing rather than falling back to the plan-time hooks.
func TestPreparedConfigurationCarriesApplyTimeExecution(t *testing.T) {
	got, err := preparedConfiguration(roundTrip(t, executionResponse(t, &pluginv1.ComponentExecutionConfiguration{
		BeforeHooks: []*pluginv1.ComponentHook{{Name: "plan_only", Commands: []string{"plan"}, Execute: []string{"tool", "plan"}}},
		Apply: &pluginv1.ComponentExecutionConfiguration{
			AfterHooks: []*pluginv1.ComponentHook{{Name: "notify", Commands: []string{"apply"}, Execute: []string{"tool", "notify"}}},
		},
	})))
	require.NoError(t, err)
	require.Equal(t, []schema.ComponentHook{{Name: "plan_only", Commands: []string{"plan"}, Execute: []string{"tool", "plan"}}}, got.Execution.BeforeHooks)
	require.Equal(t, &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{{Name: "notify", Commands: []string{"apply"}, Execute: []string{"tool", "notify"}}},
	}, got.Execution.Apply)
	require.Same(t, got.Execution.Apply, got.Execution.ForSavedPlanApply())

	// Apply adds nothing, while plan adds nothing either: the empty evaluation still wins.
	got, err = preparedConfiguration(roundTrip(t, executionResponse(t, &pluginv1.ComponentExecutionConfiguration{Apply: &pluginv1.ComponentExecutionConfiguration{}})))
	require.NoError(t, err)
	require.NotNil(t, got.Execution)
	require.True(t, got.Execution.IsEmpty())
	require.Equal(t, &schema.ComponentExecution{}, got.Execution.ForSavedPlanApply())

	// Without an apply-time evaluation, a saved plan applies the plan-time metadata.
	got, err = preparedConfiguration(roundTrip(t, executionResponse(t, &pluginv1.ComponentExecutionConfiguration{
		AfterHooks: []*pluginv1.ComponentHook{{Name: "always", Commands: []string{"apply"}, Execute: []string{"tool"}}},
	})))
	require.NoError(t, err)
	require.Same(t, got.Execution, got.Execution.ForSavedPlanApply())
}

// The unit's errors block is not command specific, so a saved plan applied with the
// apply-time metadata keeps the retry and ignore policies of the reviewed preparation.
func TestPreparedApplyTimeExecutionKeepsErrorPolicies(t *testing.T) {
	response := executionResponse(t, &pluginv1.ComponentExecutionConfiguration{
		Apply: &pluginv1.ComponentExecutionConfiguration{
			AfterHooks: []*pluginv1.ComponentHook{{Name: "notify", Commands: []string{"apply"}, Execute: []string{"tool", "notify"}}},
		},
	})
	response.Errors = &pluginv1.ComponentErrorsConfiguration{
		Retry: []*pluginv1.ComponentRetryPolicy{{Name: "net", RetryableErrors: []string{"timeout"}, MaxAttempts: 3, SleepIntervalSeconds: 1}},
	}
	got, err := preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.NotNil(t, got.Execution.Errors)
	applied := got.Execution.ForSavedPlanApply()
	require.Same(t, got.Execution.Apply, applied)
	require.Equal(t, got.Execution.Errors, applied.Errors)
	require.Equal(t, "net", applied.Errors.Retry[0].Name)

	// Without errors policies, the apply-time metadata has none either.
	response.Errors = nil
	got, err = preparedConfiguration(roundTrip(t, response))
	require.NoError(t, err)
	require.Nil(t, got.Execution.Errors)
	require.Nil(t, got.Execution.ForSavedPlanApply().Errors)
}
