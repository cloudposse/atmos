package cloudformation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// everything written to it. The package's I/O layer resolves os.Stdout/
// os.Stderr dynamically at write time (see testmain_test.go), so this simple
// swap is sufficient to observe ui.Writeln/ui.Error output.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	fn()

	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

// normalizeUIOutput strips ANSI styling and collapses whitespace (including the
// soft line-wraps toastMarkdown inserts at narrower terminal widths) so content
// assertions don't depend on the rendering width of the environment running them.
func normalizeUIOutput(out string) string {
	return strings.Join(strings.Fields(ansi.Strip(out)), " ")
}

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

// printStackEvent must render a plain transition line via ui.Writeln for a
// non-failed status, including the status reason when present.
func TestPrintStackEvent_NonFailedStatus(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
	}

	out := captureStderr(t, func() { printStackEvent(event) })
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, "AWS::S3::Bucket")
	assert.Contains(t, out, string(cfntypes.ResourceStatusCreateComplete))
	assert.NotContains(t, out, " — ", "no reason set: the em-dash suffix must not appear")
}

// printStackEvent must append the status reason (when set) and route through
// ui.Error (still on stderr) for a FAILED status.
func TestPrintStackEvent_FailedStatusWithReason(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	reason := "Bucket already exists"
	event := &cfntypes.StackEvent{
		LogicalResourceId:    &logicalID,
		ResourceType:         &resourceType,
		ResourceStatus:       cfntypes.ResourceStatusCreateFailed,
		ResourceStatusReason: &reason,
	}

	out := normalizeUIOutput(captureStderr(t, func() { printStackEvent(event) }))
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, string(cfntypes.ResourceStatusCreateFailed))
	assert.Contains(t, out, "Bucket already exists")
}

// pollStackEvents must propagate a non-"not found" DescribeStackEvents error
// rather than treating it as a deleted stack.
func TestPollStackEvents_DescribeStackEventsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, _, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "access denied")
}

// pollStackEvents must propagate a non-"not found" DescribeStacks error,
// still returning any freshly-seen events collected before the failure.
func TestPollStackEvents_DescribeStacksErrorNonNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	eventID := "event-1"
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{{EventId: &eventID}},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	events, status, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.Error(t, err)
	assert.Empty(t, status)
	assert.Len(t, events, 1, "events fetched before the DescribeStacks failure must still be returned")
}

// streamStackEvents must propagate a pollStackEvents failure immediately.
func TestStreamStackEvents_PollError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := streamStackEvents(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// streamStackEvents must return ctx.Err() when the context is cancelled
// while waiting between polls of a still-in-progress stack.
func TestStreamStackEvents_ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateInProgress}},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled: the select must take the ctx.Done() branch immediately.

	status, err := streamStackEvents(ctx, client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, cfntypes.StackStatusCreateInProgress, status, "the last-observed (non-terminal) status must still be returned")
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
