package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOperationSummary_ChangesetNameReachesCI(t *testing.T) {
	tests := []struct {
		operation Operation
		event     hooks.HookEvent
	}{
		{operation: OperationDiff, event: hooks.AfterAwsCloudFormationDiff},
		{operation: OperationApply, event: hooks.AfterAwsCloudFormationApply},
	}
	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			var createdName string
			expectChangesetCIFlow(t, client, tt.operation, &createdName)
			octx := &opContext{Ctx: context.Background(), AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}, Flags: map[string]any{"format": "json"}}
			summary, err := operationHandlers[tt.operation](octx, client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{"stack_name": "vpc"})
			require.NoError(t, err)
			require.NotEmpty(t, createdName)
			assert.Equal(t, "changeset-id-from-aws", summary["changeset_id"])

			original := runCIHooks
			t.Cleanup(func() { runCIHooks = original })
			var captured *hooks.RunCIHooksOptions
			runCIHooks = func(opts *hooks.RunCIHooksOptions) error { captured = opts; return nil }
			runCIHook(ciHookParams{event: tt.event, summary: summary})
			require.NotNil(t, captured)
			result, ok := captured.Aggregate.(*schema.CloudFormationCIResult)
			require.True(t, ok)
			assert.Equal(t, createdName, result.ChangeSetName, "CI must show the actual generated name sent to AWS, not the changeset ID")
		})
	}
}

func expectChangesetCIFlow(t *testing.T, client *MockCloudFormationClient, operation Operation, createdName *string) {
	t.Helper()
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
		*createdName = stringValue(input.ChangeSetName)
		return &cloudformation.CreateChangeSetOutput{}, nil
	})
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete, ChangeSetId: awsString("changeset-id-from-aws"), Changes: []cfntypes.Change{{Type: cfntypes.ChangeTypeResource}},
	}, nil)
	if operation == OperationDiff {
		client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil)
		return
	}
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{{EventId: awsString("new-event"), ResourceStatus: cfntypes.ResourceStatusCreateComplete}}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{Outputs: []cfntypes.Output{{OutputKey: awsString("VpcId"), OutputValue: awsString("vpc-123")}}}}}, nil),
	)
}
