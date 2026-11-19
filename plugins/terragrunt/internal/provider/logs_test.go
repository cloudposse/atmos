package provider

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// The sensitive parts of the request values are redacted from the events of the call, both in a response
// and in a failure, while their other parts stay readable.
func TestLogsRedactSensitiveRequestValues(t *testing.T) {
	configuration, err := component.Encode(cty.ObjectVal(map[string]cty.Value{
		"vars": cty.ObjectVal(map[string]cty.Value{
			"password": cty.StringVal("pw-789").Mark(component.Sensitive),
			"ports":    cty.TupleVal([]cty.Value{cty.NumberIntVal(8443)}).Mark(component.Sensitive),
			"name":     cty.StringVal("visible"),
		}),
	}))
	require.NoError(t, err)
	options, err := encode(map[string]any{"token": "not-sensitive"})
	require.NoError(t, err)

	_, logs := (&Provider{}).withLogs(t.Context(), &pluginv1.Invocation{LogLevel: "debug"}, "", configuration, options, nil)
	logger := log.New(log.WithOutput(io.Discard), log.WithHooks(logs.sink.Hook()))
	require.NoError(t, logger.SetLevel(logs.sink.TerragruntLevel()))
	logger.Debug("pw-789 8443 visible not-sensitive")

	entries := logs.entries()
	require.Len(t, entries, 1)
	require.Equal(t, "<MASKED> <MASKED> visible not-sensitive", entries[0].GetMessage())

	err = withFailureLogs(rpcError(&model.Problem{Code: model.CodeUnsupportedSetting, Summary: "failed", Safe: true, Err: ErrUnsupported}), logs)
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	require.Len(t, detail.Logs, 1)
	require.Equal(t, "<MASKED> <MASKED> visible not-sensitive", detail.Logs[0].GetMessage())
}

// A sensitive integer above 2^53 is registered by its exact digits, not a rounded float.
func TestLogsRedactLargeSensitiveIntegersExactly(t *testing.T) {
	large, err := cty.ParseNumberVal("9007199254740993")
	require.NoError(t, err)
	configuration, err := component.Encode(cty.ObjectVal(map[string]cty.Value{
		"vars": cty.ObjectVal(map[string]cty.Value{"id": large.Mark(component.Sensitive)}),
	}))
	require.NoError(t, err)

	_, logs := (&Provider{}).withLogs(t.Context(), &pluginv1.Invocation{LogLevel: "debug"}, "", configuration)
	logger := log.New(log.WithOutput(io.Discard), log.WithHooks(logs.sink.Hook()))
	require.NoError(t, logger.SetLevel(logs.sink.TerragruntLevel()))
	logger.Debug("account 9007199254740993")

	entries := logs.entries()
	require.Len(t, entries, 1)
	require.Equal(t, "account <MASKED>", entries[0].GetMessage())
}

func TestPrepareReturnsEngineLogsOnlyAtTheInvocationLevel(t *testing.T) {
	root := fixture(t, "explicit-stack")
	opts, err := encode(map[string]any{"enabled": true, "mode": "native", "discovery": map[string]any{"enabled": true, "included_paths": []string{"live/**"}}})
	require.NoError(t, err)
	provider := New(nil)
	discover := func(level string) *pluginv1.DiscoverComponentsResponse {
		response, err := provider.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, IncludePaths: []string{"live/**"}, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{LogLevel: level, Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}})
		require.NoError(t, err)
		return response
	}
	prepare := func(component *pluginv1.DiscoveredComponent, level string) *pluginv1.PrepareComponentResponse {
		request := &pluginv1.ComponentRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: opts, Address: component.Address, Source: component.Source, Configuration: component.Configuration, Operation: &pluginv1.ComponentOperation{Mode: "native", Command: "plan"}, Invocation: &pluginv1.Invocation{LogLevel: level, Environment: map[string]string{"ATMOS_TERRAFORM_BINARY": "must-not-execute"}}}
		request.Effects = &pluginv1.ComponentEffects{Commands: true, Network: true, Secrets: true, Writes: true, StateReads: true}
		response, err := provider.Prepare(t.Context(), request)
		require.NoError(t, err)
		return response
	}

	quiet := discover("")
	require.Empty(t, quiet.Logs, "engine chatter is hidden at the Atmos default level, even when Terragrunt warns")
	require.Empty(t, discover("off").Logs)
	noisy := discover("trace")
	require.NotEmpty(t, noisy.Logs)
	require.Empty(t, prepare(noisy.Components[1], "warn").Logs)
	logs := prepare(noisy.Components[1], "debug").Logs
	require.NotEmpty(t, logs)
	for _, entry := range logs {
		require.NotEqual(t, pluginv1.LogLevel_LOG_LEVEL_UNSPECIFIED, entry.Level)
		require.NotEqual(t, pluginv1.LogLevel_LOG_LEVEL_TRACE, entry.Level, "trace events are below the requested level")
		require.NotEmpty(t, entry.Message)
		require.NotNil(t, entry.Time)
	}
}

func TestFailureCarriesTheLogsThatPrecededIt(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "module"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "terragrunt.hcl"), []byte("terraform { source = \"./module\" }\ninputs = { broken = local.absent }\n"), 0o644))
	request := componentRequest(t)
	request.RootDirectory, request.Source = root, "terragrunt.hcl"
	request.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	request.Invocation.LogLevel = "trace"

	_, err = New(nil).Prepare(t.Context(), request)

	require.Error(t, err)
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	require.NotEmpty(t, detail.Logs, "the events before the failure explain it")
	request.Invocation.LogLevel = "off"
	_, err = New(nil).Prepare(t.Context(), request)
	detail, ok = component.FailureDetail(err)
	require.True(t, ok)
	require.Empty(t, detail.Logs)
}

func TestLogLevelsMapOntoTheProtocol(t *testing.T) {
	for name, want := range map[string]pluginv1.LogLevel{
		"trace": pluginv1.LogLevel_LOG_LEVEL_TRACE, "DEBUG": pluginv1.LogLevel_LOG_LEVEL_DEBUG, "info": pluginv1.LogLevel_LOG_LEVEL_INFO,
		"warn": pluginv1.LogLevel_LOG_LEVEL_WARN, "error": pluginv1.LogLevel_LOG_LEVEL_ERROR, "other": pluginv1.LogLevel_LOG_LEVEL_UNSPECIFIED,
	} {
		require.Equal(t, want, logLevel(name), name)
	}
}

// A sensitive operation option is registered before evaluation, so an event that carries it is masked
// in preparation and in state resolution, while an option that is not marked stays readable.
func TestLogsRedactSensitiveOperationOptions(t *testing.T) {
	token, err := component.Encode(cty.StringVal("tok-456").Mark(component.Sensitive))
	require.NoError(t, err)
	plain, err := component.Encode(cty.StringVal("visible-option"))
	require.NoError(t, err)
	request := &pluginv1.ComponentRequest{
		Invocation: &pluginv1.Invocation{LogLevel: "debug"},
		Operation:  &pluginv1.ComponentOperation{Options: map[string]*pluginv1.ComponentValue{"iam_assume_role_web_identity_token": token, "download_dir": plain}},
	}

	_, logs := (&Provider{}).withLogs(t.Context(), request.GetInvocation(), "", requestValues(request)...)
	logger := log.New(log.WithOutput(io.Discard), log.WithHooks(logs.sink.Hook()))
	require.NoError(t, logger.SetLevel(logs.sink.TerragruntLevel()))
	logger.Debug("tok-456 visible-option")

	entries := logs.entries()
	require.Len(t, entries, 1)
	require.Equal(t, "<MASKED> visible-option", entries[0].GetMessage())
	require.Len(t, requestValues(&pluginv1.ComponentRequest{}), 2, "a request without options still lists its two configurations")
}
