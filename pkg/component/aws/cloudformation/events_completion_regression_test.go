package cloudformation

import (
	"context"
	"errors"
	"testing"
	"time"

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

// testStackLevelName is the stack every stack-level test event belongs to.
const testStackLevelName = "vpc"

// stackLevelEvent builds the event the test stack emits for its own status change.
func stackLevelEvent(id string, status cfntypes.ResourceStatus) cfntypes.StackEvent {
	stackName := testStackLevelName
	return cfntypes.StackEvent{
		EventId:           awsString(id),
		StackId:           awsString("arn:aws:cloudformation:us-east-2:123456789012:stack/" + stackName + "/guid"),
		LogicalResourceId: awsString(stackName),
		ResourceType:      awsString(stackResourceType),
		ResourceStatus:    status,
	}
}

// The events are read before the stack status, so a stack that finishes between
// the two reads used to lose its final stack-level event (the lane saw a missing
// CREATE_COMPLETE in 1 of about 12 applies). After a terminal status is
// accepted, the events are read once more, and the late event is printed.
func TestStreamStackEvents_FinalStackEventArrivingAfterPollIsPrinted(t *testing.T) {
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })

	client := NewMockCloudFormationClient(gomock.NewController(t))
	inProgress := stackLevelEvent("e1", cfntypes.ResourceStatusCreateInProgress)
	complete := stackLevelEvent("e2", cfntypes.ResourceStatusCreateComplete)
	gomock.InOrder(
		// Poll 1: the stack is still creating.
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{inProgress}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateInProgress}}}, nil),
		// Poll 2: events were read before CREATE_COMPLETE existed, the status after.
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{inProgress}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}}}, nil),
		// Final read: the stack-level CREATE_COMPLETE is there.
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{complete, inProgress}}, nil),
	)

	var status cfntypes.StackStatus
	var err error
	out := captureStderr(t, func() {
		status, err = streamStackEvents(context.Background(), client, "vpc", eventBaseline{valid: true}, OperationApply)
	})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusCreateComplete, status)
	assert.Contains(t, out, "vpc ("+stackResourceType+"): CREATE_COMPLETE")
}

// Negative path: when the poll already showed the stack's own terminal event,
// nothing is missing, so no extra read is made (the mock has no third call).
func TestStreamStackEvents_NoFinalReadWhenStackTerminalEventSeen(t *testing.T) {
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })

	client := NewMockCloudFormationClient(gomock.NewController(t))
	complete := stackLevelEvent("e2", cfntypes.ResourceStatusCreateComplete)
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{complete}}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}}}, nil),
	)
	status, err := streamStackEvents(context.Background(), client, "vpc", eventBaseline{valid: true}, OperationApply)
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusCreateComplete, status)
}

// After a delete finishes, the stack cannot be read by name any more, so its
// final DELETE_COMPLETE event used to be lost (the stream stopped at
// DELETE_IN_PROGRESS). It is read by the stack ID recorded in the baseline.
func TestStreamStackEvents_DeleteCompleteReadByStackID(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	notFound := errors.New("Stack [vpc] does not exist")
	deleted := stackLevelEvent("e9", cfntypes.ResourceStatusDeleteComplete)
	stackID := *deleted.StackId
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("vpc")}).Return(nil, notFound),
		client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString(stackID)}).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{deleted}}, nil),
	)

	var status cfntypes.StackStatus
	var err error
	out := captureStderr(t, func() {
		status, err = streamStackEvents(context.Background(), client, "vpc", eventBaseline{valid: true, stackID: stackID, seen: map[string]bool{}}, OperationDelete)
	})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusDeleteComplete, status)
	assert.Contains(t, out, "vpc ("+stackResourceType+"): DELETE_COMPLETE")
}

// preOperationEventBaseline records the stack ID from the existing events.
func TestPreOperationEventBaseline_RecordsStackID(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	event := stackLevelEvent("e1", cfntypes.ResourceStatusCreateComplete)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{event}}, nil)

	baseline := preOperationEventBaseline(context.Background(), client, "vpc")
	assert.Equal(t, *event.StackId, baseline.stackID)
	assert.True(t, baseline.valid)
}
