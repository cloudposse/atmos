package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cipkg "github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/policy"
)

func TestCIContextCommand(t *testing.T) {
	restore := cipkg.SwapRegistryForTest()
	t.Cleanup(restore)
	output := &bytes.Buffer{}
	ioContext, err := iolib.NewContext(iolib.WithStreams(ciValidationTestStreams{output: output}))
	require.NoError(t, err)
	data.InitWriter(ioContext)
	t.Cleanup(data.Reset)

	run := func(args ...string) string {
		t.Helper()
		output.Reset()
		command := newContextCommand()
		command.SetArgs(args)
		require.NoError(t, command.Execute())
		return output.String()
	}
	require.JSONEq(t, `{"ci":null}`, run())
	require.JSONEq(t, `[{"path":"ci","type":"null"}]`, run("--paths"))

	cipkg.Register(github.NewProvider())
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_EVENT_NAME", "pull_request")
	t.Setenv("GITHUB_TOKEN", "never-expose-token")
	t.Setenv("PRIVATE_VARIABLE", "never-expose-private-variable")
	eventPath := filepath.Join(t.TempDir(), "event.json")
	t.Setenv("GITHUB_EVENT_PATH", eventPath)
	const event = `{"pull_request":{"head":{"repo":{"full_name":"contributor/repo"}},"base":{"repo":{"full_name":"company/repo"}}},"future_field":9007199254740993}`
	require.NoError(t, os.WriteFile(eventPath, []byte(event), 0o600))
	result := run()
	var document struct {
		CI policy.CISnapshot `json:"ci"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &document))
	require.Equal(t, github.PolicySnapshotSchema, document.CI.Schema)
	require.JSONEq(t, event, string(document.CI.Event.Data))
	require.Equal(t, policy.Available, document.CI.Event.Status)
	require.Empty(t, document.CI.Facts)
	require.NotContains(t, result, "never-expose")

	paths := run("--paths")
	require.Contains(t, paths, `ci[\"event\"][\"data\"][\"pull_request\"][\"head\"][\"repo\"][\"full_name\"]`)
	require.NotContains(t, paths, "contributor/repo")
	require.NotContains(t, paths, "9007199254740993")

	// An oversized inventory must fail without writing a partial JSON result.
	largeEvent := `{"items":[` + strings.Repeat("null,", contextMaxFields-1) + `null]}`
	require.NoError(t, os.WriteFile(eventPath, []byte(largeEvent), 0o600))
	output.Reset()
	limited := newContextCommand()
	limited.SetContext(t.Context())
	require.NoError(t, limited.Flags().Set("paths", "true"))
	require.ErrorIs(t, runCIContext(limited, nil), errContextTooManyFields)
	require.Empty(t, output.String())
	// The limit applies only to the path inventory, not the native snapshot.
	require.NoError(t, json.Unmarshal([]byte(run()), &document))
	require.JSONEq(t, largeEvent, string(document.CI.Event.Data))

	// Inspection preserves failures as evidence; it does not claim a policy passed.
	require.NoError(t, os.WriteFile(eventPath, []byte(`{`), 0o600))
	require.NoError(t, json.Unmarshal([]byte(run()), &document))
	require.Equal(t, policy.Incomplete, document.CI.Event.Status)
	require.False(t, document.CI.Event.Complete)
	require.NotEmpty(t, document.CI.Event.Reason)
	t.Setenv("GITHUB_EVENT_PATH", "")
	require.NoError(t, json.Unmarshal([]byte(run()), &document))
	require.Equal(t, policy.Unavailable, document.CI.Event.Status)

	command := newContextCommand()
	command.SetArgs([]string{"unexpected"})
	require.Error(t, command.Execute())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	command.SetContext(ctx)
	require.ErrorIs(t, runCIContext(command, nil), context.Canceled)
	require.Error(t, runCIContext(&cobra.Command{}, nil))
}

func TestCIContextPaths(t *testing.T) {
	snapshot := &policy.CISnapshot{Event: policy.Dataset{Data: json.RawMessage(`{
		"empty": {}, "items": [null, true, 1, "private-value", []], "odd.key\"\n": "value"
	}`)}}
	fields, err := ciContextPaths(snapshot)
	require.NoError(t, err)
	for _, field := range []ciContextField{
		{`ci["event"]["data"]["empty"]`, "object"},
		{`ci["event"]["data"]["items"]`, "array"},
		{`ci["event"]["data"]["items"][0]`, "null"},
		{`ci["event"]["data"]["items"][1]`, "boolean"},
		{`ci["event"]["data"]["items"][2]`, "number"},
		{`ci["event"]["data"]["items"][3]`, "string"},
		{`ci["event"]["data"]["items"][4]`, "array"},
		{`ci["event"]["data"]["odd.key\"\n"]`, "string"},
	} {
		require.Contains(t, fields, field)
		request := &policy.Request{Kind: policy.Command, Input: json.RawMessage(`{}`), CI: snapshot}
		matched, err := (&policy.CEL{}).Evaluate(t.Context(), policy.Evaluation{
			Request: request, Expression: field.Path + " == " + field.Path,
		})
		require.NoError(t, err)
		require.True(t, matched)
		matched, err = (&policy.Starlark{}).Evaluate(t.Context(), policy.Evaluation{
			Request: request, Expression: "value = ctx." + field.Path + "\nreturn True",
		})
		require.NoError(t, err)
		require.True(t, matched)
	}
	again, err := ciContextPaths(snapshot)
	require.NoError(t, err)
	require.Equal(t, fields, again, "field order must be stable")
	snapshot.Event.Data = json.RawMessage(`{`)
	_, err = ciContextPaths(snapshot)
	require.Error(t, err)
	snapshot.Event.Data = json.RawMessage(strings.Repeat(`{"nested":`, 65) + `null` + strings.Repeat(`}`, 65))
	_, err = ciContextPaths(snapshot)
	require.ErrorIs(t, err, errContextTooDeep)
}

func TestCIContextFieldLimit(t *testing.T) {
	for _, count := range []int{contextMaxFields - 1, contextMaxFields} {
		fields := []ciContextField{}
		err := walkCIContext(make([]any, count), "ci", 0, &fields)
		if count < contextMaxFields {
			require.NoError(t, err)
			require.Len(t, fields, contextMaxFields, "the root counts toward the limit")
		} else {
			require.ErrorIs(t, err, errContextTooManyFields)
			require.Len(t, fields, contextMaxFields, "the inventory must never grow beyond its limit")
		}
	}

	snapshot := &policy.CISnapshot{Event: policy.Dataset{Data: json.RawMessage(
		`{"items":[` + strings.Repeat("null,", contextMaxFields-1) + `null]}`,
	)}}
	fields, err := ciContextPaths(snapshot)
	require.ErrorIs(t, err, errContextTooManyFields)
	require.Nil(t, fields, "an incomplete inventory must not be returned")
}
