package cloudformation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
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
// Returns the final stack status. On a non-TTY stream (CI, non-interactive), plain
// event lines are printed; live per-resource spinners are a TTY-only enhancement
// the standard I/O layer degrades automatically — this function only needs to emit
// lines, not manage its own TTY detection.
func streamStackEvents(ctx context.Context, client CloudFormationClient, stackName string) (cfntypes.StackStatus, error) {
	defer perf.Track(nil, "cloudformation.streamStackEvents")()

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
			printStackEvent(&events[i])
		}

		seenInProgress = seenInProgress || isInProgressStatus(poll.Status)
		if acceptTerminalStatus(poll, seenInProgress) {
			return poll.Status, nil
		}

		if time.Now().After(deadline) {
			return poll.Status, fmt.Errorf("%w: timed out watching stack events", errUtils.ErrAwsCloudFormationChangeSetFailed)
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

// printStackEvent renders one CREATE_IN_PROGRESS -> CREATE_COMPLETE-style transition
// line on the UI channel (stderr) — see docs/io-and-ui-output.md.
func printStackEvent(event *cfntypes.StackEvent) {
	logicalID := stringValue(event.LogicalResourceId)
	resourceType := stringValue(event.ResourceType)
	status := string(event.ResourceStatus)
	reason := stringValue(event.ResourceStatusReason)

	line := fmt.Sprintf("%s (%s): %s", logicalID, resourceType, status)
	if reason != "" {
		line += " — " + reason
	}

	if strings.Contains(status, "FAILED") {
		ui.Error(line)
		return
	}
	ui.Writeln(line)
}
