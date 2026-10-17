package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDescribeStackOutputs(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	vpcID := "VpcId"
	vpcVal := "vpc-0123456789"
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{
				{OutputKey: &vpcID, OutputValue: &vpcVal},
			},
		}},
	}, nil)

	outputs, err := describeStackOutputs(context.Background(), client, "vpc")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-0123456789"}, outputs)
}

func TestDescribeStackOutputs_NoStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	outputs, err := describeStackOutputs(context.Background(), client, "vpc")
	require.NoError(t, err)
	assert.Empty(t, outputs)
}

// TestRunOutputMasksDeployedNoEcho requires both emitted output and the returned summary to redact
// deployed NoEcho references while retaining public values.
func TestRunOutputMasksDeployedNoEcho(t *testing.T) {
	t.Cleanup(iolib.Reset)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{Outputs: []cfntypes.Output{
		{OutputKey: awsString("Secret"), OutputValue: awsString("field-noecho-canary")},
		{OutputKey: awsString("Safe"), OutputValue: awsString("public-value")},
	}}}}, nil)
	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.GetTemplateOutput{TemplateBody: awsString(`Parameters:
  Password: {Type: String, NoEcho: true}
Outputs:
  Secret: {Value: !Sub 'prefix-${Password}'}
  Safe: {Value: public-value}
`)}, nil).AnyTimes()
	out := captureStdout(t, func() {
		summary, err := runOutput(context.Background(), client, "deployed", map[string]any{"format": "json"}, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "<MASKED>", summary["outputs"].(map[string]any)["Secret"])
	})
	assert.NotContains(t, out, "field-noecho-canary")
	assert.Contains(t, out, "public-value")
}

// TestMaskStackOutputsExpressions covers direct and indirect intrinsic references plus known secret
// values without mutating dependency outputs.
func TestMaskStackOutputsExpressions(t *testing.T) {
	for _, expression := range []string{
		"!Ref Password", "{Ref: Password}", "!Sub 'prefix-${Password}'",
		"{'Fn::Join': [':', [prefix, {Ref: Password}]]}",
		"!GetAtt SecretResource.Value", "!If [SecretCondition, one, two]",
	} {
		t.Run(expression, func(t *testing.T) {
			iolib.Reset()
			t.Cleanup(iolib.Reset)
			body := `Parameters:
  Password: {Type: String, NoEcho: true, Default: default-canary}
  Public: {Type: String}
Conditions:
  SecretCondition: !Equals [!Ref Password, expected]
Resources:
  SecretResource:
    Type: AWS::SSM::Parameter
    Properties: {Value: !Ref Password}
Outputs:
  Secret: {Value: ` + expression + `}
  Safe: {Value: visible}
  Default: {Value: literal}
  Resolved: {Value: literal}
`
			raw := map[string]any{"Secret": "old-canary", "Safe": "visible", "Default": "default-canary", "Resolved": "resolved-canary"}
			masked, err := maskStackOutputs(body, []cfntypes.Parameter{
				{ParameterKey: awsString("Password"), ParameterValue: awsString("resolved-canary")},
			}, raw)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"Secret": "<MASKED>", "Safe": "visible", "Default": "<MASKED>", "Resolved": "<MASKED>"}, masked)
			assert.Equal(t, "old-canary", raw["Secret"], "presentation must not mutate dependency values")
		})
	}
}

// TestMaskStackOutputsInvalidMetadata requires malformed sensitivity metadata to return an error
// without exposing output values.
func TestMaskStackOutputsInvalidMetadata(t *testing.T) {
	for _, body := range []string{"", "not-an-object", "[one, two]", "[invalid", "Parameters: []", "Outputs: invalid"} {
		t.Run(body, func(t *testing.T) {
			outputs, err := maskStackOutputs(body, nil, map[string]any{"Secret": "unprinted-canary"})
			require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
			assert.Nil(t, outputs)
			assert.NotContains(t, err.Error(), "unprinted-canary")
		})
	}
}

// TestPresentedStackOutputsMetadataFailureAndOptOut requires metadata failures to suppress output
// unless masking was explicitly disabled.
func TestPresentedStackOutputsMetadataFailureAndOptOut(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			iolib.Reset()
			t.Cleanup(iolib.Reset)
			iolib.GetContext().Masker().SetEnabled(enabled)
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{Outputs: []cfntypes.Output{{OutputKey: awsString("Secret"), OutputValue: awsString("canary")}}}}}, nil)
			if enabled {
				client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))
			}
			out := captureStdout(t, func() {
				summary, err := runOutput(context.Background(), client, "deployed", map[string]any{"format": "json"}, map[string]any{})
				if enabled {
					require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
					assert.Contains(t, err.Error(), "cloudformation:GetTemplate")
					assert.NotContains(t, summary, "outputs")
				} else {
					require.NoError(t, err)
					assert.Equal(t, "canary", summary["outputs"].(map[string]any)["Secret"])
				}
			})
			if enabled {
				assert.Empty(t, out)
			} else {
				assert.Contains(t, out, "canary")
			}
		})
	}
}

// TestOutputMaskingFreshProcess verifies every output format with no previously registered secrets,
// using deployed NoEcho metadata alone.
func TestOutputMaskingFreshProcess(t *testing.T) {
	if format := os.Getenv("_ATMOS_TEST_CF_OUTPUT"); format != "" {
		client := NewMockCloudFormationClient(gomock.NewController(t))
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{Parameters: []cfntypes.Parameter{{ParameterKey: awsString("Password"), ParameterValue: awsString("****")}}, Outputs: []cfntypes.Output{
			{OutputKey: awsString("Secret"), OutputValue: awsString("fresh-process-canary")},
			{OutputKey: awsString("Safe"), OutputValue: awsString("public-value")},
		}}}}, nil)
		client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.GetTemplateOutput{TemplateBody: awsString(`{"Parameters":{"Password":{"Type":"String","NoEcho":true}},"Outputs":{"Secret":{"Value":{"Ref":"Password"}},"Safe":{"Value":"public-value"}}}`)}, nil)
		_, err := runOutput(context.Background(), client, "deployed", map[string]any{"format": format}, map[string]any{})
		require.NoError(t, err)
		return
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	for _, format := range []string{"json", "yaml", "table", "env", "hcl", "csv", "tsv", "github"} {
		t.Run(format, func(t *testing.T) {
			cmd := exec.Command(exe, "-test.run=^TestOutputMaskingFreshProcess$")
			cmd.Env = append(os.Environ(), "_ATMOS_TEST_CF_OUTPUT="+format)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			assert.NotContains(t, string(output), "fresh-process-canary")
			assert.Contains(t, string(output), "public-value")
		})
	}
}

// TestMaskStackOutputsMissingOutputMetadata redacts outputs whose sensitivity cannot be established
// while preserving the raw dependency map.
func TestMaskStackOutputsMissingOutputMetadata(t *testing.T) {
	iolib.Reset()
	t.Cleanup(iolib.Reset)
	raw := map[string]any{"Unknown": "unknown-sensitivity-canary"}
	actual, err := maskStackOutputs("Resources: {}", nil, raw)
	require.NoError(t, err)
	assert.Equal(t, "<MASKED>", actual["Unknown"])
	assert.Equal(t, "unknown-sensitivity-canary", raw["Unknown"])
}
