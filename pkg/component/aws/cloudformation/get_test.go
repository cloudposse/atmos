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
)

// getDeployedTemplate must request TemplateStageOriginal only when original
// is true, and the processed (default) stage otherwise.
func TestGetDeployedTemplate_OriginalVsProcessed(t *testing.T) {
	tests := []struct {
		name        string
		original    bool
		wantStage   cfntypes.TemplateStage
		wantNoneSet bool
	}{
		{"original stage", true, cfntypes.TemplateStageOriginal, false},
		{"default (processed) stage", false, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)

			var gotStage cfntypes.TemplateStage
			client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, input *cloudformation.GetTemplateInput, _ ...func(*cloudformation.Options)) (*cloudformation.GetTemplateOutput, error) {
					gotStage = input.TemplateStage
					return &cloudformation.GetTemplateOutput{TemplateBody: awsString("Resources: {}")}, nil
				},
			)

			body, err := getDeployedTemplate(context.Background(), client, "vpc", tt.original)
			require.NoError(t, err)
			assert.Equal(t, "Resources: {}", body)
			if tt.wantNoneSet {
				assert.Empty(t, gotStage, "the zero-value TemplateStage must be left unset (processed, CloudFormation's default) when original is false")
			} else {
				assert.Equal(t, tt.wantStage, gotStage)
			}
		})
	}
}

// getDeployedTemplate must wrap a GetTemplate API error.
func TestGetDeployedTemplate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := getDeployedTemplate(context.Background(), client, "vpc", false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// getDeployedStackPolicy must return the policy body on success, including
// "" when the stack has no policy set (CloudFormation's own contract — no
// error in that case).
func TestGetDeployedStackPolicy_Success(t *testing.T) {
	tests := []struct {
		name string
		body *string
		want string
	}{
		{"policy set", awsString(`{"Statement": []}`), `{"Statement": []}`},
		{"no policy set", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)
			client.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(&cloudformation.GetStackPolicyOutput{
				StackPolicyBody: tt.body,
			}, nil)

			body, err := getDeployedStackPolicy(context.Background(), client, "vpc")
			require.NoError(t, err)
			assert.Equal(t, tt.want, body)
		})
	}
}

// getDeployedStackPolicy must wrap a GetStackPolicy API error.
func TestGetDeployedStackPolicy_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := getDeployedStackPolicy(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runGetTemplate must write the template body to the data channel and
// populate the summary, honoring the --original flag.
func TestRunGetTemplate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	var gotStage cfntypes.TemplateStage
	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.GetTemplateInput, _ ...func(*cloudformation.Options)) (*cloudformation.GetTemplateOutput, error) {
			gotStage = input.TemplateStage
			return &cloudformation.GetTemplateOutput{TemplateBody: awsString("AWSTemplateFormatVersion: '2010-09-09'")}, nil
		},
	)

	out := captureStdout(t, func() {
		summary, err := runGetTemplate(context.Background(), client, "vpc", map[string]any{"original": true}, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "AWSTemplateFormatVersion: '2010-09-09'", summary["template"])
	})
	assert.Equal(t, cfntypes.TemplateStageOriginal, gotStage)
	assert.Equal(t, "AWSTemplateFormatVersion: '2010-09-09'", out, "--original must print the body exactly as returned")
}

// --original must be byte-exact: comments, quoting, key order, indentation and
// the absence of a trailing newline all survive. The processed (default) form
// is still re-serialized, so this guards the difference between the two.
func TestRunGetTemplate_OriginalIsByteExact(t *testing.T) {
	body := "# A leading comment.\nResources:\n    Bucket:   {Type: 'AWS::S3::Bucket'}\nOutputs: {}"

	tests := []struct {
		name         string
		flags        map[string]any
		wantExact    bool
		wantContains string
	}{
		{"original is verbatim", map[string]any{"original": true}, true, ""},
		{"processed is re-serialized", map[string]any{}, false, "Resources:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(&cloudformation.GetTemplateOutput{TemplateBody: awsString(body)}, nil)

			out := captureStdout(t, func() {
				_, err := runGetTemplate(context.Background(), client, "vpc", tt.flags, map[string]any{})
				require.NoError(t, err)
			})
			if tt.wantExact {
				assert.Equal(t, body, out)
				return
			}
			assert.NotEqual(t, body, out)
			assert.Contains(t, out, tt.wantContains)
		})
	}
}

// A missing stack on a get verb is the stack-not-found sentinel with a hint,
// not AWS's raw validation message. Other failures keep the API sentinel.
func TestGetVerbs_MissingStackMapsToStackNotFound(t *testing.T) {
	missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}

	tests := []struct {
		name string
		call func(client CloudFormationClient) error
		mock func(client *MockCloudFormationClient, err error)
	}{
		{
			name: "get policy",
			call: func(c CloudFormationClient) error {
				_, err := getDeployedStackPolicy(context.Background(), c, "vpc")
				return err
			},
			mock: func(c *MockCloudFormationClient, err error) {
				c.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(nil, err)
			},
		},
		{
			name: "get template",
			call: func(c CloudFormationClient) error {
				_, err := getDeployedTemplate(context.Background(), c, "vpc", false)
				return err
			},
			mock: func(c *MockCloudFormationClient, err error) {
				c.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(nil, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" missing stack", func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			tt.mock(client, missing)

			err := tt.call(client)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotFound)
			require.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
			hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
			assert.Contains(t, hints, "atmos aws cloudformation apply")
			assert.NotContains(t, hints, "--target")
		})
		t.Run(tt.name+" other error", func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			tt.mock(client, errors.New("throttled"))

			err := tt.call(client)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
			require.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotFound)
		})
	}
}

// runGetTemplate must pretty-print the body as YAML even when CloudFormation
// returns it as JSON (its common stored representation regardless of how the
// template was originally authored), rather than echoing AWS's raw response.
func TestRunGetTemplate_JSONBodyPrettyPrintedAsYAML(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(
		&cloudformation.GetTemplateOutput{
			TemplateBody: awsString(`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{"Marker":{"Type":"AWS::SSM::Parameter"}}}`),
		}, nil,
	)

	out := captureStdout(t, func() {
		summary, err := runGetTemplate(context.Background(), client, "vpc", map[string]any{}, map[string]any{})
		require.NoError(t, err)
		formatted, ok := summary["template"].(string)
		require.True(t, ok)
		assert.NotContains(t, formatted, "{", "JSON body must be re-serialized as YAML, not echoed verbatim")
		assert.Contains(t, formatted, "AWSTemplateFormatVersion:")
		assert.Contains(t, formatted, "Resources:")
		assert.Contains(t, formatted, "Marker:")
	})
	assert.NotContains(t, out, "{", "the data channel must receive the pretty-printed YAML, not raw JSON")
}

// runGetTemplate must propagate a getDeployedTemplate failure.
func TestRunGetTemplate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetTemplate(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	_, err := runGetTemplate(context.Background(), client, "vpc", map[string]any{}, map[string]any{})
	require.Error(t, err)
}

// runGetPolicy must write the policy body to the data channel when set.
func TestRunGetPolicy_PolicySet(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(&cloudformation.GetStackPolicyOutput{
		StackPolicyBody: awsString(`{"Statement": []}`),
	}, nil)

	out := captureStdout(t, func() {
		summary, err := runGetPolicy(context.Background(), client, "vpc", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, `{"Statement": []}`, summary["stack_policy"])
	})
	assert.Contains(t, out, `{"Statement": []}`)
}

// runGetPolicy must render a "no stack policy set" line, not the raw body,
// when the stack has no policy — the branch this test targets.
func TestRunGetPolicy_NoPolicySet(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(&cloudformation.GetStackPolicyOutput{}, nil)

	out := captureStdout(t, func() {
		summary, err := runGetPolicy(context.Background(), client, "vpc", map[string]any{})
		require.NoError(t, err)
		assert.Empty(t, summary["stack_policy"])
	})
	assert.Contains(t, out, "vpc: no stack policy set")
}

// runGetPolicy must propagate a getDeployedStackPolicy failure.
func TestRunGetPolicy_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().GetStackPolicy(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runGetPolicy(context.Background(), client, "vpc", map[string]any{})
	require.Error(t, err)
}
