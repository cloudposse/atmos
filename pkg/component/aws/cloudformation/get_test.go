package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
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
	assert.Contains(t, out, "AWSTemplateFormatVersion")
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
