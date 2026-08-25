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
// completes (or fails), returning the overall stack drift status.
func detectDrift(ctx context.Context, client CloudFormationClient, stackName string) (*driftDetectionResult, error) {
	defer perf.Track(nil, "cloudformation.detectDrift")()

	out, err := client.DetectStackDrift(ctx, &cloudformation.DetectStackDriftInput{StackName: awsString(stackName)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationDriftDetected, err)
	}

	detectionID := stringValue(out.StackDriftDetectionId)
	return pollDriftDetection(ctx, client, detectionID)
}

// pollDriftDetection polls DescribeStackDriftDetectionStatus until the detection
// operation reaches DETECTION_COMPLETE or DETECTION_FAILED.
func pollDriftDetection(ctx context.Context, client CloudFormationClient, detectionID string) (*driftDetectionResult, error) {
	deadline := time.Now().Add(driftDetectionTimeout)
	for {
		out, err := client.DescribeStackDriftDetectionStatus(ctx, &cloudformation.DescribeStackDriftDetectionStatusInput{
			StackDriftDetectionId: awsString(detectionID),
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationDriftDetected, err)
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
			return result, fmt.Errorf("%w: %s", errUtils.ErrAwsCloudFormationDriftDetected, result.StatusReason)
		}

		if time.Now().After(deadline) {
			return result, fmt.Errorf("%w: timed out waiting for drift detection", errUtils.ErrAwsCloudFormationDriftDetected)
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(driftPollInterval):
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
			return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationDriftDetected, err)
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
