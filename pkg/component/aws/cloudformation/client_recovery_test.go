package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRecoveryPollDoesNotRepeatExecutedChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.error.code == 'Throttling'")
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, &smithy.GenericAPIError{Code: "Throttling", Message: "slow down"}),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
	)
	original := operationHandlers[OperationApply]
	t.Cleanup(func() { operationHandlers[OperationApply] = original })
	operationHandlers[OperationApply] = func(octx *opContext, client CloudFormationClient, _ *stackSpec, summary map[string]any) (map[string]any, error) {
		if _, err := client.ExecuteChangeSet(octx.Ctx, &cloudformation.ExecuteChangeSetInput{}); err != nil {
			return nil, err
		}
		_, err := client.DescribeStacks(octx.Ctx, &cloudformation.DescribeStacksInput{})
		return summary, err
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	_, err := runRemoteOperation(&opContext{Ctx: context.Background(), Info: info}, OperationApply, client, &stackSpec{}, map[string]any{})
	require.NoError(t, err)
}
