package cloudformation

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// driftPollInterval is how often DescribeStackDriftDetectionStatus is polled
// while a drift-detection operation is in progress. A var (not const) so
// tests can shrink it and exercise the polling loop without a real 3s sleep.
var driftPollInterval = 3 * time.Second

// driftDetectionTimeout bounds how long a single drift detection may run before
// this command gives up watching it. A var (not const) so tests can shrink it
// to exercise the timeout branch without waiting 15 real minutes.
var driftDetectionTimeout = 15 * time.Minute

// driftDetectionResult is the outcome of running (or re-fetching the status of)
// a drift detection operation.
type driftDetectionResult struct {
	DetectionID   string
	StackStatus   cfntypes.StackDriftStatus
	DriftedCount  int32
	StatusReason  string
	DetectionDone bool
}

// detectDrift starts a new drift detection operation and polls until it
// completes (or fails), returning the overall stack drift status. The
// driftDetectionTimeout budget is applied once, here, before the initial
// DetectStackDrift call, and its deadline is shared with pollDriftDetection.
// A stalled startup request is bounded by the same 15-minute budget as the
// polling loop that follows it, instead of only the polling loop being
// bounded. The original ctx is preserved and threaded through so caller
// cancellation is still reported distinctly from a timeoutCtx deadline.
func detectDrift(ctx context.Context, client CloudFormationClient, stackName string) (*driftDetectionResult, error) {
	defer perf.Track(nil, "cloudformation.detectDrift")()

	timeoutCtx, cancel := context.WithTimeout(ctx, driftDetectionTimeout)
	defer cancel()

	out, err := client.DetectStackDrift(timeoutCtx, &cloudformation.DetectStackDriftInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, classifyDriftPollError(ctx, timeoutCtx, err)
	}

	detectionID := stringValue(out.StackDriftDetectionId)
	return pollDriftDetection(ctx, timeoutCtx, client, detectionID)
}

// errDriftDetectionTimedOut wraps ErrAwsCloudFormationAPICallFailed for both places
// pollDriftDetection can discover the per-detection timeout: a stalled AWS request unblocked by
// timeoutCtx's deadline, and the between-poll wait's own timeoutCtx.Done() case.
func errDriftDetectionTimedOut() error {
	return fmt.Errorf("%w: timed out waiting for drift detection", errUtils.ErrAwsCloudFormationAPICallFailed)
}

// classifyDriftPollError maps a DescribeStackDriftDetectionStatus failure to the right error:
// the caller's own cancellation (ctx.Err()) takes priority, then the per-request timeoutCtx
// deadline, then the raw AWS error wrapped generically. Split out of pollDriftDetection to keep
// its cyclomatic complexity down.
func classifyDriftPollError(ctx, timeoutCtx context.Context, err error) error {
	if parentErr := ctx.Err(); parentErr != nil {
		return parentErr
	}
	if timeoutCtx.Err() != nil {
		return errDriftDetectionTimedOut()
	}
	return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
}

// waitForNextDriftPoll blocks until the next poll is due, returning nil to continue looping.
// The caller's own context (ctx) is checked first, with priority: once both ctx and the derived
// timeoutCtx are done (timeoutCtx is cancelled whenever its parent ctx is), a select between
// them could otherwise pick either case at random, sometimes misreporting a caller cancellation
// as a drift-detection timeout.
func waitForNextDriftPoll(ctx, timeoutCtx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timeoutCtx.Done():
		return errDriftDetectionTimedOut()
	case <-time.After(driftPollInterval):
		return nil
	}
}

// pollDriftDetection polls DescribeStackDriftDetectionStatus until the detection operation
// reaches DETECTION_COMPLETE or DETECTION_FAILED. The timeoutCtx (created by detectDrift, before
// the initial DetectStackDrift call) bounds every AWS request in this loop too, so a stalled
// DescribeStackDriftDetectionStatus call can't keep the command blocked past the shared
// driftDetectionTimeout budget. The caller's own cancellation (ctx.Err()) is preserved and
// reported separately from the child context's own deadline, which maps to
// ErrAwsCloudFormationAPICallFailed.
func pollDriftDetection(ctx, timeoutCtx context.Context, client CloudFormationClient, detectionID string) (*driftDetectionResult, error) {
	for {
		out, err := client.DescribeStackDriftDetectionStatus(timeoutCtx, &cloudformation.DescribeStackDriftDetectionStatusInput{
			StackDriftDetectionId: awsString(detectionID),
		})
		if err != nil {
			return nil, classifyDriftPollError(ctx, timeoutCtx, err)
		}

		result := &driftDetectionResult{
			DetectionID:  detectionID,
			StackStatus:  out.StackDriftStatus,
			StatusReason: stringValue(out.DetectionStatusReason),
		}
		if out.DriftedStackResourceCount != nil {
			result.DriftedCount = *out.DriftedStackResourceCount
		}

		switch out.DetectionStatus {
		case cfntypes.StackDriftDetectionStatusDetectionComplete:
			result.DetectionDone = true
			return result, nil
		case cfntypes.StackDriftDetectionStatusDetectionFailed:
			return result, fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationAPICallFailed, result.StatusReason)
		}

		if err := waitForNextDriftPoll(ctx, timeoutCtx); err != nil {
			return result, err
		}
	}
}

// describeResourceDrifts fetches the per-resource drift details from the most
// recently completed drift detection for the stack (does not trigger a new
// detection — pair with detectDrift/runDriftDetect first for a fresh check).
func describeResourceDrifts(ctx context.Context, client CloudFormationClient, stackName string) ([]cfntypes.StackResourceDrift, error) {
	defer perf.Track(nil, "cloudformation.describeResourceDrifts")()

	var drifts []cfntypes.StackResourceDrift
	var nextToken *string
	for {
		out, err := client.DescribeStackResourceDrifts(ctx, &cloudformation.DescribeStackResourceDriftsInput{
			StackName: awsString(stackName),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
		}
		drifts = append(drifts, out.StackResourceDrifts...)
		if out.NextToken == nil {
			return drifts, nil
		}
		nextToken = out.NextToken
	}
}

// runDriftDetect triggers a fresh drift detection and renders the summary result.
// Returns ErrAwsCloudFormationDriftDetected when drift is found and failOnDrift is
// set, so callers can wire it to a non-zero exit code (e.g. `--fail-on-drift` in CI).
func runDriftDetect(ctx context.Context, client CloudFormationClient, stackName string, failOnDrift bool, summary map[string]any) (map[string]any, error) {
	result, err := detectDrift(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["drift_status"] = string(result.StackStatus)
	summary["drifted_resource_count"] = result.DriftedCount

	_ = data.Writeln(fmt.Sprintf("%s: %s (%d resource(s) drifted)", stackName, result.StackStatus, result.DriftedCount))

	if failOnDrift && result.StackStatus == cfntypes.StackDriftStatusDrifted {
		return summary, fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationDriftDetected, stackName)
	}
	return summary, nil
}

// runDriftDescribe renders the per-resource results of the most recent drift
// detection without triggering a new one.
func runDriftDescribe(ctx context.Context, client CloudFormationClient, stackName string, summary map[string]any) (map[string]any, error) {
	drifts, err := describeResourceDrifts(ctx, client, stackName)
	if err != nil {
		return summary, err
	}
	summary["drifts"] = drifts

	if len(drifts) == 0 {
		_ = data.Writeln(fmt.Sprintf("%s: no drift results (run drift detect first)", stackName))
		return summary, nil
	}
	for i := range drifts {
		d := &drifts[i]
		if d.StackResourceDriftStatus == cfntypes.StackResourceDriftStatusInSync {
			continue
		}
		line := fmt.Sprintf("  %-10s %-28s %s", d.StackResourceDriftStatus, stringValue(d.ResourceType), stringValue(d.LogicalResourceId))
		_ = data.Writeln(line)
	}
	return summary, nil
}
