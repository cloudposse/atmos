package provider

import (
	"io"
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

	_, sink := withLogs(t.Context(), &pluginv1.Invocation{LogLevel: "debug"}, "", configuration, options, nil)
	logger := log.New(log.WithOutput(io.Discard), log.WithHooks(sink.Hook()))
	require.NoError(t, logger.SetLevel(sink.TerragruntLevel()))
	logger.Debug("pw-789 8443 visible not-sensitive")

	entries := logEntries(sink)
	require.Len(t, entries, 1)
	require.Equal(t, "<MASKED> <MASKED> visible not-sensitive", entries[0].GetMessage())

	err = withFailureLogs(rpcError(&model.Problem{Code: model.CodeUnsupportedSetting, Summary: "failed", Safe: true, Err: ErrUnsupported}), sink)
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

	_, sink := withLogs(t.Context(), &pluginv1.Invocation{LogLevel: "debug"}, "", configuration)
	logger := log.New(log.WithOutput(io.Discard), log.WithHooks(sink.Hook()))
	require.NoError(t, logger.SetLevel(sink.TerragruntLevel()))
	logger.Debug("account 9007199254740993")

	entries := logEntries(sink)
	require.Len(t, entries, 1)
	require.Equal(t, "account <MASKED>", entries[0].GetMessage())
}
