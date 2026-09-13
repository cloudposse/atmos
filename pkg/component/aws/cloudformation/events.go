package cloudformation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// eventPollInterval is how often DescribeStackEvents is polled while a stack
// operation (create/update/delete) is in progress. A var (not const) so tests
// can shrink it to avoid real-time sleeps.
var eventPollInterval = 3 * time.Second

// operationTimeout bounds how long a single stack create/update/delete may run
// before this command gives up watching it (the operation itself may continue in
// the account; CloudFormation's own timeout_in_minutes governs that).
const operationTimeout = 60 * time.Minute

// stackPoll bundles one pollStackEvents observation: the stack's current
// status, and whether that status was learned via an unambiguous "the stack
// is actually gone" signal (Gone) as opposed to an ordinary DescribeStacks
// status read (which can be stale/leftover from a previous operation; see
// acceptTerminalStatus).
type stackPoll struct {
	Status cfntypes.StackStatus
	Gone   bool
}

// streamStackEvents polls DescribeStackEvents from the moment it's called and
// prints each new event as it appears, until the stack reaches a terminal status.
// Returns the final stack status. On an interactive TTY, in-progress resources
// animate a live spinner line (via pkg/ui/spinner.Spinner) while completed/failed
// resources are printed permanently above it, colored by outcome; on a non-TTY
// stream (CI, non-interactive), plain event lines are printed via printStackEvent,
// unchanged from before.
func streamStackEvents(ctx context.Context, client CloudFormationClient, stackName string) (cfntypes.StackStatus, error) {
	defer perf.Track(nil, "cloudformation.streamStackEvents")()

	var sp *spinner.Spinner
	if term.IsTTYSupportForStdout() {
		sp = spinner.New(fmt.Sprintf("%s: watching stack events…", stackName))
		sp.Start()
		// Belt-and-suspenders: Stop is idempotent, so this guarantees the spinner
		// is never left running on every return path (poll error, ctx
		// cancellation, timeout), even though the terminal-status path below
		// already stops it via Success/Error.
		defer sp.Stop()
	}

	seen := make(map[string]bool)
	deadline := time.Now().Add(operationTimeout)

	// seenInProgress guards against misreading a stack's leftover terminal
	// status from a previous, unrelated operation as this operation's
	// completion: ExecuteChangeSet/DeleteStack return before CloudFormation
	// applies the change, so the very next DescribeStacks call can still
	// return the pre-execution status. Once we've observed a `*_IN_PROGRESS`
	// status for this operation, a subsequent terminal status is trustworthy.
	seenInProgress := false

	for {
		events, poll, err := pollStackEvents(ctx, client, stackName, seen)
		if err != nil {
			return "", err
		}
		for i := range events {
			dispatchStackEvent(sp, &events[i])
		}

		seenInProgress = seenInProgress || isInProgressStatus(poll.Status)
		if acceptTerminalStatus(poll, seenInProgress) {
			finishStreamSpinner(sp, stackName, poll.Status)
			return poll.Status, nil
		}

		if time.Now().After(deadline) {
			return poll.Status, fmt.Errorf("%w: timed out watching stack events", errUtils.ErrAwsCloudFormationOperationFailed)
		}
		select {
		case <-ctx.Done():
			return poll.Status, ctx.Err()
		case <-time.After(eventPollInterval):
		}
	}
}

// isInProgressStatus reports whether status is a non-empty `*_IN_PROGRESS` status.
func isInProgressStatus(status cfntypes.StackStatus) bool {
	return status != "" && strings.HasSuffix(string(status), "_IN_PROGRESS")
}

// acceptTerminalStatus reports whether poll's status may be treated as this
// operation's completion. A poll.Gone signal (the stack is confirmed to have
// actually disappeared) is a positive, unambiguous signal accepted regardless
// of seenInProgress. Any other terminal status must first have been preceded
// by an observed `*_IN_PROGRESS` status, to rule out reading a stale,
// pre-execution terminal status (left over from an earlier, unrelated
// operation) as this operation's completion.
func acceptTerminalStatus(poll stackPoll, seenInProgress bool) bool {
	return poll.Status != "" && isTerminalStackStatus(poll.Status) && (poll.Gone || seenInProgress)
}

// dispatchStackEvent routes one stack event either to the plain non-TTY line
// printer (sp == nil) or to the live spinner: in-progress events update the
// spinner's live line, while terminal (complete/failed) events are printed
// permanently above it, colored by outcome.
func dispatchStackEvent(sp *spinner.Spinner, event *cfntypes.StackEvent) {
	if sp == nil {
		printStackEvent(event)
		return
	}

	line, _ := formatStackEventLine(event, true)
	status := cfntypes.StackStatus(event.ResourceStatus)

	switch {
	case !isTerminalStackStatus(status):
		sp.Update(line)
	case isFailedStackStatus(status):
		sp.Println(ui.FormatError(line))
	default:
		sp.Println(ui.FormatSuccess(line))
	}
}

// finishStreamSpinner reports the stack's final terminal status on the spinner
// (success or error, matching the resource-level coloring above) and stops it.
// A no-op when sp is nil (non-TTY path never started one). The stack name is
// bolded (markdown, rendered by Spinner.Success/Error) to match
// dispatchStackEvent's per-resource lines instead of flat text.
func finishStreamSpinner(sp *spinner.Spinner, stackName string, status cfntypes.StackStatus) {
	if sp == nil {
		return
	}

	line := fmt.Sprintf("**%s**: %s", stackName, status)
	if isFailedStackStatus(status) {
		sp.Error(line)
		return
	}
	sp.Success(line)
}

// pollStackEvents fetches the current stack status and any events not already in
// seen, oldest-first (the API returns newest-first). Returns an empty status when
// the stack has been fully deleted (DescribeStacks returns not-found), with
// stackPoll.Gone set -- see stackPoll and acceptTerminalStatus.
func pollStackEvents(ctx context.Context, client CloudFormationClient, stackName string, seen map[string]bool) ([]cfntypes.StackEvent, stackPoll, error) {
	eventsOut, err := client.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{StackName: awsString(stackName)})
	if err != nil {
		// A delete can complete (and the stack disappear) faster than this poll loop's
		// first iteration — observed against Floci, which drops a deleted stack's event
		// history immediately rather than retaining it the way real AWS does. Treat "not
		// found" here the same as the DescribeStacks not-found check below: the stack is
		// gone, which is delete's successful terminal state, not an error.
		if isStackNotFoundError(err) {
			return nil, stackPoll{Status: cfntypes.StackStatusDeleteComplete, Gone: true}, nil
		}
		return nil, stackPoll{}, err
	}

	var fresh []cfntypes.StackEvent
	for i := len(eventsOut.StackEvents) - 1; i >= 0; i-- {
		event := eventsOut.StackEvents[i]
		id := stringValue(event.EventId)
		if seen[id] {
			continue
		}
		seen[id] = true
		fresh = append(fresh, event)
	}

	stacksOut, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{StackName: awsString(stackName)})
	if err != nil {
		if isStackNotFoundError(err) {
			return fresh, stackPoll{Status: cfntypes.StackStatusDeleteComplete, Gone: true}, nil
		}
		return fresh, stackPoll{}, err
	}
	if len(stacksOut.Stacks) == 0 {
		return fresh, stackPoll{Status: cfntypes.StackStatusDeleteComplete, Gone: true}, nil
	}
	return fresh, stackPoll{Status: stacksOut.Stacks[0].StackStatus}, nil
}

// isTerminalStackStatus reports whether a stack status is a resting state (not a
// `*_IN_PROGRESS` transition).
func isTerminalStackStatus(status cfntypes.StackStatus) bool {
	return !strings.HasSuffix(string(status), "_IN_PROGRESS")
}

// isFailedStackStatus reports whether a terminal stack status indicates the
// operation did not succeed.
func isFailedStackStatus(status cfntypes.StackStatus) bool {
	s := string(status)
	return strings.Contains(s, "FAILED") || strings.Contains(s, "ROLLBACK")
}

// formatStackEventLine renders one CREATE_IN_PROGRESS -> CREATE_COMPLETE-style
// transition line, plus whether the event represents a failure. Pure formatting,
// no I/O — callers pick the output channel (see printStackEvent for the UI/stderr
// channel watch uses, and writeLogLine for the data/stdout channel logs uses).
//
// The markdown parameter must be true only for callers whose line flows into a
// renderer that actually processes markdown (ui.FormatSuccess/FormatError/
// FormatInline/Info — see dispatchStackEvent): those bold the logical ID and
// code-span the resource type for a lightly formatted TTY line instead of flat
// text. Plain data/log channels (ui.Writeln, data.Writeln) never render
// markdown, so passing true there would leak literal "**"/backtick characters
// into piped or CI output.
func formatStackEventLine(event *cfntypes.StackEvent, markdown bool) (line string, failed bool) {
	logicalID := stringValue(event.LogicalResourceId)
	resourceType := stringValue(event.ResourceType)
	status := string(event.ResourceStatus)
	reason := stringValue(event.ResourceStatusReason)

	if markdown {
		line = fmt.Sprintf("**%s** (`%s`): %s", logicalID, resourceType, status)
	} else {
		line = fmt.Sprintf("%s (%s): %s", logicalID, resourceType, status)
	}
	if reason != "" {
		line += " — " + reason
	}
	return line, strings.Contains(status, "FAILED")
}

// printStackEvent renders one stack event on the UI channel (stderr) — see
// docs/io-and-ui-output.md. Used by watch, which is live human-facing status,
// not pipeable data.
func printStackEvent(event *cfntypes.StackEvent) {
	line, failed := formatStackEventLine(event, false)
	if failed {
		ui.Error(line)
		return
	}
	ui.Writeln(line)
}
