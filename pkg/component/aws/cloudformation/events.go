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
// operation (create/update/delete) is in progress.
const eventPollInterval = 3 * time.Second

// operationTimeout bounds how long a single stack create/update/delete may run
// before this command gives up watching it (the operation itself may continue in
// the account; CloudFormation's own timeout_in_minutes governs that).
const operationTimeout = 60 * time.Minute

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

	for {
		events, status, err := pollStackEvents(ctx, client, stackName, seen)
		if err != nil {
			return "", err
		}
		for i := range events {
			printStackEvent(&events[i])
		}

		if status != "" && isTerminalStackStatus(status) {
			return status, nil
		}

		if time.Now().After(deadline) {
			return status, fmt.Errorf("%w: timed out watching stack events", errUtils.ErrAwsCloudFormationChangeSetFailed)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(eventPollInterval):
		}
	}
}

// pollStackEvents fetches the current stack status and any events not already in
// seen, oldest-first (the API returns newest-first). Returns an empty status when
// the stack has been fully deleted (DescribeStacks returns not-found).
func pollStackEvents(ctx context.Context, client CloudFormationClient, stackName string, seen map[string]bool) ([]cfntypes.StackEvent, cfntypes.StackStatus, error) {
	eventsOut, err := client.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, "", err
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
			return fresh, cfntypes.StackStatusDeleteComplete, nil
		}
		return fresh, "", err
	}
	if len(stacksOut.Stacks) == 0 {
		return fresh, cfntypes.StackStatusDeleteComplete, nil
	}
	return fresh, stacksOut.Stacks[0].StackStatus, nil
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
	ui.Write(line)
}
