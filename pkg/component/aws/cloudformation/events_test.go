package cloudformation

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ui/spinner"
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
	// DescribeStackEvents returns the same event on both polls (CloudFormation's
	// API always returns the full event history, not just new events since the
	// last call) -- pollStackEvents' own seen-map bookkeeping is what must
	// prevent the second call from re-reporting event-1 as fresh.
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: &eventID1, LogicalResourceId: &logicalID, ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		},
	}, nil).Times(2)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateInProgress}},
	}, nil).Times(2)

	seen := make(map[string]bool)
	events, poll, err := pollStackEvents(context.Background(), client, "vpc", seen)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, cfntypes.StackStatusCreateInProgress, poll.Status)
	assert.False(t, poll.Gone)
	assert.True(t, seen["event-1"])

	// Second poll with the same client and seen map: event-1 was already
	// recorded, so it must not be reported as fresh again.
	events, poll, err = pollStackEvents(context.Background(), client, "vpc", seen)
	require.NoError(t, err)
	assert.Empty(t, events, "an event already recorded in seen must not be reported as fresh on a subsequent poll")
	assert.Equal(t, cfntypes.StackStatusCreateInProgress, poll.Status)
	assert.False(t, poll.Gone)
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

// formatStackEventLine(markdown=true) must bold the logical ID and code-span
// the resource type, for dispatchStackEvent's TTY line (which flows into
// ui.FormatSuccess/FormatError/FormatInline — renderers that actually process
// markdown, see the function's own doc comment).
func TestFormatStackEventLine_MarkdownWrapsIdentifiers(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
	}

	line, failed := formatStackEventLine(event, true)
	assert.False(t, failed)
	assert.Contains(t, line, "**MyBucket**")
	assert.Contains(t, line, "`AWS::S3::Bucket`")
}

// formatStackEventLine(markdown=false) must never contain markdown syntax —
// printStackEvent and writeLogLine route this straight to ui.Writeln/
// data.Writeln, which write raw text with no markdown rendering; leaking "**"
// or backticks there would corrupt piped/CI output.
func TestFormatStackEventLine_PlainHasNoMarkdown(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
	}

	line, _ := formatStackEventLine(event, false)
	assert.NotContains(t, line, "**")
	assert.NotContains(t, line, "`")
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

	events, poll, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.Error(t, err)
	assert.Empty(t, poll.Status)
	assert.False(t, poll.Gone)
	assert.Len(t, events, 1, "events fetched before the DescribeStacks failure must still be returned")
}

// streamStackEvents must propagate a pollStackEvents failure immediately.
func TestStreamStackEvents_PollError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := streamStackEvents(context.Background(), client, "vpc", map[string]bool{})
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

	status, err := streamStackEvents(ctx, client, "vpc", map[string]bool{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, cfntypes.StackStatusCreateInProgress, status, "the last-observed (non-terminal) status must still be returned")
}

// streamStackEvents must not treat a terminal status observed on its very
// first poll as completion when that status was never preceded by an
// observed `*_IN_PROGRESS` status, AND the poll's only event was already
// present in the pre-operation baseline (not genuinely new):
// ExecuteChangeSet/DeleteStack return before CloudFormation applies the
// change, so the first DescribeStacks call can still return a leftover
// terminal status from a previous, unrelated operation -- and
// DescribeStackEvents always returns the stack's full event history, so that
// same previous operation's last event reappears on this poll too. Accepting
// either signal here would misreport "done" before the requested operation
// even started (the original CodeRabbit-flagged race). The event is seeded
// into both the mocked poll response and the pre-operation baseline passed in,
// proving pollStackEvents' dedup (via the shared seen map) — not just an
// empty StackEvents list — is what keeps seenNewEvent from firing on a stale
// event. This test pre-cancels the context so that once the loop correctly
// declines to return on the stale terminal status, the next select() picks
// the ctx.Done() branch instead of sleeping a real eventPollInterval.
func TestStreamStackEvents_IgnoresStaleTerminalStatusWithoutInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// A single poll returning a terminal status left over from a previous
	// operation, with no `*_IN_PROGRESS` status ever observed, and whose only
	// event is the same one already recorded in the pre-operation baseline.
	staleEventID := "stale-event-1"
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{{EventId: &staleEventID}},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled: if the loop wrongly returns success, we'd never reach the select.

	// Pre-seeded baseline: this event already existed before the operation
	// started, so it must not be reported as fresh (see pollStackEvents), and
	// therefore must not set seenNewEvent either.
	baseline := map[string]bool{staleEventID: true}
	status, err := streamStackEvents(ctx, client, "vpc", baseline)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled,
		"a stale terminal status with no observed IN_PROGRESS and no genuinely new event must not be accepted as completion")
	assert.Equal(t, cfntypes.StackStatusUpdateComplete, status)
}

// streamStackEvents must accept a terminal status on its very first poll when
// that poll's events include one absent from the pre-operation baseline (see
// preOperationEventBaseline), even though no `*_IN_PROGRESS` status was ever
// observed. This is the fast-completion race CodeRabbit flagged on PR #3157:
// a sufficiently fast create/update can reach its terminal status between
// ExecuteChangeSet returning and this loop's very first poll, so relying on
// seenInProgress alone would spin until operationTimeout and report a false
// timeout despite the operation having already finished successfully.
func TestStreamStackEvents_AcceptsTerminalStatusOnFirstPollWithNewEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	newEventID := "new-event-1"
	logicalID := "MyBucket"
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: &newEventID, LogicalResourceId: &logicalID, ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)

	// Empty baseline: nothing existed on the stack before this operation (a
	// brand-new CREATE), so the single event returned on this very first poll
	// is unambiguously new -- no *_IN_PROGRESS status is ever observed.
	status, err := streamStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusCreateComplete, status)
}

// streamStackEvents must accept a terminal status once an `*_IN_PROGRESS`
// status has been observed for this operation — the normal, non-racy path.
func TestStreamStackEvents_AcceptsTerminalStatusAfterObservedInProgress(t *testing.T) {
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateInProgress}},
		}, nil),
	)
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
		}, nil),
	)

	status, err := streamStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusUpdateComplete, status)
}

// preOperationEventBaseline must collect every existing event ID from
// DescribeStackEvents so streamStackEvents can seed its dedup map with them.
func TestPreOperationEventBaseline_ReturnsExistingEventIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	eventID1, eventID2 := "event-1", "event-2"
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{{EventId: &eventID1}, {EventId: &eventID2}},
	}, nil)

	seen := preOperationEventBaseline(context.Background(), client, "vpc")
	assert.True(t, seen["event-1"])
	assert.True(t, seen["event-2"])
	assert.Len(t, seen, 2)
}

// preOperationEventBaseline must degrade to an empty (non-nil) baseline --
// never an error -- when the stack doesn't exist yet: the normal case for a
// brand-new CREATE, which has no prior events at all.
func TestPreOperationEventBaseline_StackNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("Stack [vpc] does not exist"))

	seen := preOperationEventBaseline(context.Background(), client, "vpc")
	assert.NotNil(t, seen)
	assert.Empty(t, seen)
}

// preOperationEventBaseline must also degrade to an empty baseline -- never
// propagate the error -- for any other DescribeStackEvents failure (e.g. a
// transient throttle/network blip): losing the fast-path new-event signal for
// this one operation is preferable to aborting the create/update/delete that's
// about to run over it.
func TestPreOperationEventBaseline_OtherErrorDegradesGracefully(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	seen := preOperationEventBaseline(context.Background(), client, "vpc")
	assert.NotNil(t, seen)
	assert.Empty(t, seen)
}

// followLogs must propagate a pollStackEvents failure immediately, the same
// as streamStackEvents.
func TestFollowLogs_PollError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := followLogs(context.Background(), client, "vpc", []string{"vpc"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}

// followLogs must return a nil error (not ctx.Err()) on cancellation — unlike
// streamStackEvents, --follow's ctx.Done() is the expected tail -f style exit
// (Ctrl+C), not an abnormal interruption of an in-progress operation.
func TestFollowLogs_ContextCancelledReturnsNilAfterOnePoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("vpc")}).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("e1"), LogicalResourceId: awsString("Vpc"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("vpc")}).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled: one poll round happens, then the select must take ctx.Done().

	out := captureStdout(t, func() {
		summary, err := followLogs(ctx, client, "vpc", []string{"vpc"}, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 1, summary["event_count"])
	})
	// logs is a data command (docs/io-and-ui-output.md): --follow must write to
	// stdout the same as the one-shot path, never stderr.
	assert.Contains(t, out, "Vpc")
}

// followLogs must poll every stack in names independently, each with its own
// dedup set, and merge the event count across all of them.
func TestFollowLogs_PollsEveryStackIndependently(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("r1"), LogicalResourceId: awsString("RootResource"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)
	client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("child")}).Return(&cloudformation.DescribeStackEventsOutput{
		StackEvents: []cfntypes.StackEvent{
			{EventId: awsString("c1"), LogicalResourceId: awsString("ChildResource"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		},
	}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("child")}).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	out := captureStdout(t, func() {
		summary, err := followLogs(ctx, client, "root", []string{"root", "child"}, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, 2, summary["event_count"])
	})
	assert.Contains(t, out, "RootResource")
	assert.Contains(t, out, "ChildResource")
}

// dispatchStackEvent must route an in-progress event to Spinner.Update, which
// degrades to ui.Info off-TTY (the test harness isn't a TTY, so spinner.New
// here produces a non-interactive Spinner exercising that degraded path).
func TestDispatchStackEvent_InProgressUpdatesSpinner(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateInProgress,
	}

	sp := spinner.New("watching")
	out := normalizeUIOutput(captureStderr(t, func() { dispatchStackEvent(sp, event) }))
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, string(cfntypes.ResourceStatusCreateInProgress))
}

// dispatchStackEvent must route a completed (non-failed) terminal event to
// Spinner.Println with FormatSuccess coloring (checkmark icon), not a plain line.
func TestDispatchStackEvent_CompleteUsesFormatSuccess(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
	}

	sp := spinner.New("watching")
	out := normalizeUIOutput(captureStderr(t, func() { dispatchStackEvent(sp, event) }))
	assert.Contains(t, out, "✓", "terminal, non-failed events must render via ui.FormatSuccess")
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, string(cfntypes.ResourceStatusCreateComplete))
}

// dispatchStackEvent must route a failed terminal event to Spinner.Println
// with FormatError coloring (X icon), including the status reason.
func TestDispatchStackEvent_FailedUsesFormatError(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	reason := "Bucket already exists"
	event := &cfntypes.StackEvent{
		LogicalResourceId:    &logicalID,
		ResourceType:         &resourceType,
		ResourceStatus:       cfntypes.ResourceStatusCreateFailed,
		ResourceStatusReason: &reason,
	}

	sp := spinner.New("watching")
	out := normalizeUIOutput(captureStderr(t, func() { dispatchStackEvent(sp, event) }))
	assert.Contains(t, out, "✗", "failed terminal events must render via ui.FormatError")
	assert.Contains(t, out, "MyBucket")
	assert.Contains(t, out, "Bucket already exists")
}

// dispatchStackEvent with a nil spinner (the non-TTY streamStackEvents path)
// must fall back to printStackEvent unchanged, never the spinner-flavored
// FormatSuccess coloring.
func TestDispatchStackEvent_NilSpinnerFallsBackToPrintStackEvent(t *testing.T) {
	logicalID := "MyBucket"
	resourceType := "AWS::S3::Bucket"
	event := &cfntypes.StackEvent{
		LogicalResourceId: &logicalID,
		ResourceType:      &resourceType,
		ResourceStatus:    cfntypes.ResourceStatusCreateComplete,
	}

	out := captureStderr(t, func() { dispatchStackEvent(nil, event) })
	assert.Contains(t, out, "MyBucket")
	assert.NotContains(t, out, "✓", "nil spinner must use plain ui.Writeln, not FormatSuccess coloring")
}

// finishStreamSpinner must report a successful terminal status via
// Spinner.Success (FormatSuccess coloring).
func TestFinishStreamSpinner_SuccessStatus(t *testing.T) {
	sp := spinner.New("watching")
	out := normalizeUIOutput(captureStderr(t, func() { finishStreamSpinner(sp, "vpc", cfntypes.StackStatusCreateComplete) }))
	assert.Contains(t, out, "✓")
	assert.Contains(t, out, "vpc")
	assert.Contains(t, out, string(cfntypes.StackStatusCreateComplete))
}

// finishStreamSpinner must report a failed terminal status via Spinner.Error
// (FormatError coloring).
func TestFinishStreamSpinner_FailedStatus(t *testing.T) {
	sp := spinner.New("watching")
	out := normalizeUIOutput(captureStderr(t, func() { finishStreamSpinner(sp, "vpc", cfntypes.StackStatusCreateFailed) }))
	assert.Contains(t, out, "✗")
	assert.Contains(t, out, "vpc")
	assert.Contains(t, out, string(cfntypes.StackStatusCreateFailed))
}

// finishStreamSpinner must be a no-op for a nil spinner (the non-TTY path
// never starts one).
func TestFinishStreamSpinner_NilSpinnerIsNoop(t *testing.T) {
	out := captureStderr(t, func() { finishStreamSpinner(nil, "vpc", cfntypes.StackStatusCreateComplete) })
	assert.Empty(t, out)
}

func TestPollStackEvents_StackDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	_, poll, err := pollStackEvents(context.Background(), client, "vpc", map[string]bool{})
	require.NoError(t, err)
	assert.Equal(t, cfntypes.StackStatusDeleteComplete, poll.Status)
	assert.True(t, poll.Gone, "an empty Stacks list from DescribeStacks is a positive, unambiguous completion signal")
}
