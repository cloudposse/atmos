package cloudformation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGeneratedBytesReachCloudFormation(t *testing.T) {
	config, info := generationFixture(t, t.TempDir(), "api")
	spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
	require.NoError(t, err)
	template, err := os.ReadFile(spec.TemplateAbsPath)
	require.NoError(t, err)
	policy, err := os.ReadFile(filepath.Join(filepath.Dir(spec.TemplateAbsPath), "policy.json"))
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}}}, nil).Times(2)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
		require.NotNil(t, input.TemplateBody)
		require.Equal(t, string(template), *input.TemplateBody)
		return &cloudformation.CreateChangeSetOutput{}, nil
	})
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{ChangeSetId: awsString("reviewed"), Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("The submitted information didn't contain changes.")}, nil)
	client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil)
	client.EXPECT().SetStackPolicy(gomock.Any(), &cloudformation.SetStackPolicyInput{StackName: awsString(spec.StackName), StackPolicyBody: awsString(string(policy))}).Return(&cloudformation.SetStackPolicyOutput{}, nil)
	_, err = runApply(&opContext{Ctx: t.Context(), AtmosConfig: config, Info: info, Flags: map[string]any{"auto-approve": true}}, client, spec, map[string]any{})
	require.NoError(t, err)
}
