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
)

func TestEventsFor(t *testing.T) {
	tests := []struct {
		operation  Operation
		wantBefore hooks.HookEvent
		wantAfter  hooks.HookEvent
	}{
		{OperationDiff, hooks.BeforeAwsCloudFormationDiff, hooks.AfterAwsCloudFormationDiff},
		{OperationApply, hooks.BeforeAwsCloudFormationApply, hooks.AfterAwsCloudFormationApply},
		{OperationDelete, hooks.BeforeAwsCloudFormationDelete, hooks.AfterAwsCloudFormationDelete},
		{OperationRender, hooks.HookEvent(""), hooks.HookEvent("")},
	}
	for _, tt := range tests {
		t.Run(string(tt.operation), func(t *testing.T) {
			before, after := eventsFor(tt.operation)
			assert.Equal(t, tt.wantBefore, before)
			assert.Equal(t, tt.wantAfter, after)
		})
	}
}

func TestDeleteOptionsFromFlags(t *testing.T) {
	opts := deleteOptionsFromFlags(map[string]any{
		"retain-resources":               []string{"MyBucket", "MyQueue"},
		"disable-termination-protection": true,
	})
	assert.Equal(t, []string{"MyBucket", "MyQueue"}, opts.RetainResources)
	assert.True(t, opts.DisableTerminationProtection)

	empty := deleteOptionsFromFlags(map[string]any{})
	assert.Empty(t, empty.RetainResources)
	assert.False(t, empty.DisableTerminationProtection)
}

func TestRunDiff(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
		Changes: []cfntypes.Change{
			{Type: cfntypes.ChangeTypeResource},
		},
	}, nil)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	summary, err := runDiff(context.Background(), client, spec, map[string]any{})
	require.NoError(t, err)
	assert.False(t, summary["no_op"].(bool))
	assert.Len(t, summary["changes"].([]cfntypes.Change), 1)
}

func TestRunDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	summary, err := runDelete(context.Background(), client, map[string]any{}, spec, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, string(cfntypes.StackStatusDeleteComplete), summary["final_status"])
}

func TestRunDelete_FailedStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteFailed}},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	_, err := runDelete(context.Background(), client, map[string]any{}, spec, map[string]any{})
	require.Error(t, err)
}

func TestRunOutput(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	outputKey := "VpcId"
	outputVal := "vpc-123"
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{{OutputKey: &outputKey, OutputValue: &outputVal}},
		}},
	}, nil)

	summary, err := runOutput(context.Background(), client, "vpc", map[string]any{"format": "json"}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-123"}, summary["outputs"])
}

func TestRunOperation_Render_NoAPICalls(t *testing.T) {
	// Render must never touch the AWS API — no mock expectations set means any
	// call would fail the test via gomock's unexpected-call panic.
	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	octx := &opContext{Ctx: context.Background()}
	summary, err := runOperation(octx, OperationRender, spec)
	require.NoError(t, err)
	assert.Equal(t, spec.TemplateBody, summary["template"])
}
