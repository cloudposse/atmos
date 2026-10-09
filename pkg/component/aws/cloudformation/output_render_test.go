package cloudformation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// expectStackWithStatus makes DescribeStacks report one stack in the given
// status carrying the given Output key/value pairs.
func expectStackWithStatus(client *MockCloudFormationClient, status cfntypes.StackStatus, outputs map[string]string) {
	stackOutputs := make([]cfntypes.Output, 0, len(outputs))
	for k, v := range outputs {
		stackOutputs = append(stackOutputs, cfntypes.Output{OutputKey: awsString(k), OutputValue: awsString(v)})
	}
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: status, Outputs: stackOutputs}},
	}, nil)
}

// disableOutputMasking turns presentation masking off for a test so it does
// not need to stub the deployed template lookup.
func disableOutputMasking(t *testing.T) {
	t.Helper()
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	iolib.GetContext().Masker().SetEnabled(false)
}

// A stack that only holds a preview changeset (or a failed or deleted create)
// has no outputs worth printing. Reporting an empty table with rc 0 made a stub
// look managed and healthy.
func TestDescribeStackOutputs_NotDeployedStatuses(t *testing.T) {
	for _, status := range []cfntypes.StackStatus{
		cfntypes.StackStatusReviewInProgress,
		cfntypes.StackStatusRollbackInProgress,
		cfntypes.StackStatusRollbackComplete,
		cfntypes.StackStatusRollbackFailed,
		cfntypes.StackStatusCreateFailed,
		cfntypes.StackStatusDeleteInProgress,
		cfntypes.StackStatusDeleteComplete,
		cfntypes.StackStatusDeleteFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			expectStackWithStatus(client, status, nil)

			outputs, err := describeStackOutputs(context.Background(), client, "stub")
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotDeployed)
			assert.Nil(t, outputs)
			assert.Contains(t, err.Error(), `"stub"`)
			assert.Contains(t, err.Error(), string(status))
		})
	}
}

// Negative path: every status that carries deployed resources still returns
// its outputs.
func TestDescribeStackOutputs_DeployedStatusesAreNotRejected(t *testing.T) {
	for _, status := range []cfntypes.StackStatus{
		cfntypes.StackStatusCreateComplete,
		cfntypes.StackStatusUpdateComplete,
		cfntypes.StackStatusUpdateRollbackComplete,
		cfntypes.StackStatusUpdateInProgress,
		cfntypes.StackStatusImportComplete,
	} {
		t.Run(string(status), func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			expectStackWithStatus(client, status, map[string]string{"VpcId": "vpc-1"})

			outputs, err := describeStackOutputs(context.Background(), client, "vpc")
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"VpcId": "vpc-1"}, outputs)
		})
	}
}

// The output verb must fail on a never-deployed stack and print nothing, in
// every format (the JSON form used to print `{}` with rc 0).
func TestRunOutput_NotDeployedStackFails(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			disableOutputMasking(t)
			client := NewMockCloudFormationClient(gomock.NewController(t))
			expectStackWithStatus(client, cfntypes.StackStatusReviewInProgress, nil)

			out := captureStdout(t, func() {
				_, err := runOutput(context.Background(), client, "stub", map[string]any{"format": format}, map[string]any{})
				require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotDeployed)
			})
			assert.Empty(t, out)
		})
	}
}

// A deployed stack with no Outputs gets a one-line notice instead of an empty
// Key/Value table; structured formats still emit a parseable empty document.
func TestRunOutput_NoOutputs(t *testing.T) {
	t.Run("table prints a notice and no table", func(t *testing.T) {
		disableOutputMasking(t)
		client := NewMockCloudFormationClient(gomock.NewController(t))
		expectStackWithStatus(client, cfntypes.StackStatusCreateComplete, nil)

		var stdout string
		stderr := captureStderr(t, func() {
			stdout = captureStdout(t, func() {
				_, err := runOutput(context.Background(), client, "plain", map[string]any{}, map[string]any{})
				require.NoError(t, err)
			})
		})
		assert.Empty(t, stdout)
		assert.Contains(t, normalizeUIOutput(stderr), "Stack plain has no outputs")
	})

	t.Run("json still prints an empty object", func(t *testing.T) {
		disableOutputMasking(t)
		client := NewMockCloudFormationClient(gomock.NewController(t))
		expectStackWithStatus(client, cfntypes.StackStatusCreateComplete, nil)

		out := captureStdout(t, func() {
			_, err := runOutput(context.Background(), client, "plain", map[string]any{"format": "json"}, map[string]any{})
			require.NoError(t, err)
		})
		assert.JSONEq(t, "{}", out)
	})
}

func TestRunOutput_SingleKey(t *testing.T) {
	outputs := map[string]string{"VpcId": "vpc-123", "Url": "https://example.test/a?b=1&c=2"}
	tests := []struct {
		name   string
		flags  map[string]any
		assert func(t *testing.T, out string)
	}{
		{
			name:  "default prints the bare value for piping",
			flags: map[string]any{"key": "VpcId"},
			assert: func(t *testing.T, out string) {
				assert.Equal(t, "vpc-123\n", out)
			},
		},
		{
			name:  "json encodes the value without HTML escaping",
			flags: map[string]any{"key": "Url", "format": "json"},
			assert: func(t *testing.T, out string) {
				assert.Equal(t, "\"https://example.test/a?b=1&c=2\"\n", out)
			},
		},
		{
			name:  "yaml encodes the value",
			flags: map[string]any{"key": "VpcId", "format": "yaml"},
			assert: func(t *testing.T, out string) {
				assert.Equal(t, "vpc-123\n", out)
			},
		},
		{
			name:  "env renders KEY=value",
			flags: map[string]any{"key": "VpcId", "format": "env", "uppercase": true},
			assert: func(t *testing.T, out string) {
				assert.Equal(t, "VPCID=vpc-123\n", out)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disableOutputMasking(t)
			client := NewMockCloudFormationClient(gomock.NewController(t))
			expectStackWithStatus(client, cfntypes.StackStatusCreateComplete, outputs)

			out := captureStdout(t, func() {
				_, err := runOutput(context.Background(), client, "vpc", tt.flags, map[string]any{})
				require.NoError(t, err)
			})
			tt.assert(t, out)
		})
	}
}

// A missing key is a sentinel error that lists what the stack does have.
func TestRunOutput_SingleKeyMissing(t *testing.T) {
	disableOutputMasking(t)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	expectStackWithStatus(client, cfntypes.StackStatusCreateComplete, map[string]string{"VpcId": "vpc-1", "Arn": "arn:x"})

	out := captureStdout(t, func() {
		_, err := runOutput(context.Background(), client, "vpc", map[string]any{"key": "Valeu"}, map[string]any{})
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOutputNotFound)
		assert.Contains(t, err.Error(), `"Valeu"`)
		assert.Contains(t, err.Error(), "Arn, VpcId")
	})
	assert.Empty(t, out)
}

// NoEcho masking must still apply when a single key is requested.
func TestRunOutput_SingleKeyKeepsNoEchoMasking(t *testing.T) {
	t.Cleanup(iolib.Reset)
	iolib.Reset()
	client := NewMockCloudFormationClient(gomock.NewController(t))
	expectStackWithStatus(client, cfntypes.StackStatusCreateComplete, map[string]string{"Secret": "single-key-canary", "Safe": "public-value"})
	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.GetTemplateOutput{TemplateBody: awsString(`Parameters:
  Password: {Type: String, NoEcho: true}
Outputs:
  Secret: {Value: !Ref Password}
  Safe: {Value: public-value}
`)}, nil)

	out := captureStdout(t, func() {
		_, err := runOutput(context.Background(), client, "deployed", map[string]any{"key": "Secret"}, map[string]any{})
		require.NoError(t, err)
	})
	assert.NotContains(t, out, "single-key-canary")
	assert.Equal(t, "<MASKED>\n", out)
}

// A bad --format names the value and lists the valid formats.
func TestOutputFormat(t *testing.T) {
	format, err := outputFormat(map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "table", string(format))

	format, err = outputFormat(map[string]any{"format": "yaml"})
	require.NoError(t, err)
	assert.Equal(t, "yaml", string(format))

	_, err = outputFormat(map[string]any{"format": "bogus"})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), `"bogus"`)
	for _, valid := range []string{"json", "yaml", "hcl", "env", "dotenv", "bash", "csv", "tsv", "table", "github"} {
		assert.Contains(t, err.Error(), valid)
	}
	assert.NotContains(t, err.Error(), "\n", "the error must stay on one line")
}

// JSON output must not HTML-escape the masking placeholder, in any shape.
func TestJSONOutputIsNotHTMLEscaped(t *testing.T) {
	disableOutputMasking(t)

	out := captureStdout(t, func() {
		require.NoError(t, renderOutputsSummary(map[string]any{"Secret": "<MASKED>", "Link": "a&b"}, map[string]any{"format": "json"}))
	})
	assert.Contains(t, out, `"Secret": "<MASKED>"`)
	assert.Contains(t, out, `"Link": "a&b"`)
	assert.NotContains(t, out, `\u003c`)
	assert.NotContains(t, out, `\u0026`)

	out = captureStdout(t, func() {
		require.NoError(t, renderOutputsSummary(map[string]any{"Secret": "<MASKED>"}, map[string]any{"format": "json", "uppercase": true}))
	})
	assert.JSONEq(t, `{"SECRET": "<MASKED>"}`, out)
}

// A read error other than "does not exist" keeps the generic API sentinel; a
// missing stack maps to stack-not-found with a hint that does not mention the
// --target flag, which the output verb does not have.
func TestDescribeStackOutputs_ErrorMapping(t *testing.T) {
	t.Run("missing stack", func(t *testing.T) {
		client := NewMockCloudFormationClient(gomock.NewController(t))
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack [vpc] does not exist"})

		_, err := describeStackOutputs(context.Background(), client, "vpc")
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotFound)
		hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
		assert.NotEmpty(t, hints)
		assert.NotContains(t, hints, "--target")
	})
	t.Run("other error", func(t *testing.T) {
		client := NewMockCloudFormationClient(gomock.NewController(t))
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

		_, err := describeStackOutputs(context.Background(), client, "vpc")
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	})
}
