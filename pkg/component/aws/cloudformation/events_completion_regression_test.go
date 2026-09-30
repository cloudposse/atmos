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
)

func TestPollStackEvents_DeleteNotFoundCompletes(t *testing.T) {
	for _, source := range []string{"events", "stack"} {
		t.Run(source, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			notFound := errors.New("Stack [vpc] does not exist")
			if source == "events" {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, notFound)
			} else {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, notFound)
			}
			status, err := streamStackEvents(context.Background(), client, "vpc", eventBaseline{}, OperationDelete)
			require.NoError(t, err)
			assert.Equal(t, cfntypes.StackStatusDeleteComplete, status)
		})
	}
}

func TestStreamStackEvents_ApplyEmptyStackDoesNotComplete(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := streamStackEvents(ctx, client, "vpc", eventBaseline{valid: true}, OperationApply)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, status)
}

func TestCompletionSignals_UnavailableBaselineStillAcceptsProgress(t *testing.T) {
	signals := completionSignals{}
	assert.False(t, signals.observe(stackPoll{Status: cfntypes.StackStatusUpdateComplete}, 2))
	assert.False(t, signals.observe(stackPoll{Status: cfntypes.StackStatusUpdateInProgress}, 0))
	assert.True(t, signals.observe(stackPoll{Status: cfntypes.StackStatusUpdateComplete}, 0))
}

func TestPollStackEvents_ApplyNotFoundIsNotCompletion(t *testing.T) {
	for _, eventsNotFound := range []bool{false, true} {
		t.Run(map[bool]string{true: "events", false: "stack"}[eventsNotFound], func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			notFound := errors.New("Stack [vpc] does not exist")
			if eventsNotFound {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, notFound)
			} else {
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, notFound)
			}
			_, poll, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{}, OperationApply)
			require.ErrorIs(t, err, notFound)
			require.False(t, poll.Gone)
		})
	}
}
