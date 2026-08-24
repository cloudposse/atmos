package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestIsTerminalStackStatus(t *testing.T) {
	assert.True(t, isTerminalStackStatus(cfntypes.StackStatusCreateComplete))
	assert.True(t, isTerminalStackStatus(cfntypes.StackStatusRollbackFailed))
	assert.False(t, isTerminalStackStatus(cfntypes.StackStatusCreateInProgress))
	assert.False(t, isTerminalStackStatus(cfntypes.StackStatusUpdateInProgress))
}

func TestIsFailedStackStatus(t *testing.T) {
	assert.True(t, isFailedStackStatus(cfntypes.StackStatusCreateFailed))
	assert.True(t, isFailedStackStatus(cfntypes.StackStatusRollbackComplete))
	assert.True(t, isFailedStackStatus(cfntypes.StackStatusUpdateRollbackFailed))
	assert.False(t, isFailedStackStatus(cfntypes.StackStatusCreateComplete))
	assert.False(t, isFailedStackStatus(cfntypes.StackStatusDeleteComplete))
}

func TestPollStackEvents_DeduplicatesAcrossCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	eventID1 := "event-1"
	logicalID := "MyBucket"
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: &eventID1, LogicalResourceId: &logicalID, ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateInProgress}},
	}, nil)

	seen := make(map[string]bool)
	events, status, err := pollStackEvents(context.Background(), client, "vpc", seen)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, cfntypes.StackStatusCreateInProgress, status)
	assert.True(t, seen["event-1"])
}

func TestPollStackEvents_StackDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	_, status, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusDeleteComplete, status)
}
