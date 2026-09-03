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

	errUtils "github.com/cloudposse/atmos/errors"
)

// shrinkDriftTiming replaces the package's poll interval/timeout with tiny
// values for the duration of a test, restoring the originals on cleanup —
// otherwise the timeout and multi-poll tests below would take real minutes.
func shrinkDriftTiming(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	origInterval := driftPollInterval
	origTimeout := driftDetectionTimeout
	driftPollInterval = interval
	driftDetectionTimeout = timeout
	t.Cleanup(func() {
		driftPollInterval = origInterval
		driftDetectionTimeout = origTimeout
	})
}

// detectDrift's happy path: start detection, then poll once to DETECTION_COMPLETE.
func TestDetectDrift_Success(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(&cloudformation.DetectStackDriftOutput{
		StackDriftDetectionId: awsString("detection-1"),
	}, nil)
	count := int32(2)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus:           cfntypes.StackDriftDetectionStatusDetectionComplete,
		StackDriftStatus:          cfntypes.StackDriftStatusDrifted,
		DriftedStackResourceCount: &count,
	}, nil)

	result, err := detectDrift(context.Background(), client, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "detection-1", result.DetectionID)
	assert.True(t, result.DetectionDone)
	assert.Equal(t, cfntypes.StackDriftStatusDrifted, result.StackStatus)
	assert.Equal(t, int32(2), result.DriftedCount)
}

// detectDrift must wrap a DetectStackDrift API error without polling.
func TestDetectDrift_StartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := detectDrift(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// pollDriftDetection must wrap a DescribeStackDriftDetectionStatus API error.
func TestPollDriftDetection_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := pollDriftDetection(context.Background(), client, "detection-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// pollDriftDetection must report DETECTION_FAILED as an error, carrying the
// status reason through.
func TestPollDriftDetection_DetectionFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus:       cfntypes.StackDriftDetectionStatusDetectionFailed,
		DetectionStatusReason: awsString("internal failure"),
	}, nil)

	result, err := pollDriftDetection(context.Background(), client, "detection-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.Contains(t, err.Error(), "internal failure")
	assert.False(t, result.DetectionDone)
}

// pollDriftDetection must keep polling (not return) while DetectionStatus is
// still IN_PROGRESS, then return once it reaches DETECTION_COMPLETE.
func TestPollDriftDetection_ContinuesUntilComplete(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	gomock.InOrder(
		client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
			DetectionStatus: cfntypes.StackDriftDetectionStatusDetectionInProgress,
		}, nil),
		client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
			DetectionStatus:  cfntypes.StackDriftDetectionStatusDetectionComplete,
			StackDriftStatus: cfntypes.StackDriftStatusInSync,
		}, nil),
	)

	result, err := pollDriftDetection(context.Background(), client, "detection-1")
	require.NoError(t, err)
	assert.True(t, result.DetectionDone)
	assert.Equal(t, cfntypes.StackDriftStatusInSync, result.StackStatus)
}

// pollDriftDetection must give up once the deadline passes, without ever
// reaching a terminal detection status.
func TestPollDriftDetection_Timeout(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Millisecond)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	// Always in-progress: the timeout, not a terminal status, must end the loop.
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus: cfntypes.StackDriftDetectionStatusDetectionInProgress,
	}, nil).AnyTimes()

	_, err := pollDriftDetection(context.Background(), client, "detection-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.Contains(t, err.Error(), "timed out")
}

// pollDriftDetection must return ctx.Err() when the context is cancelled
// while waiting between polls.
func TestPollDriftDetection_ContextCancelled(t *testing.T) {
	shrinkDriftTiming(t, time.Minute, time.Hour)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus: cfntypes.StackDriftDetectionStatusDetectionInProgress,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pollDriftDetection(ctx, client, "detection-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// describeResourceDrifts must paginate through every NextToken.
func TestDescribeResourceDrifts_Paginates(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	token := "page-2"
	gomock.InOrder(
		client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackResourceDriftsOutput{
			StackResourceDrifts: []cfntypes.StackResourceDrift{{LogicalResourceId: awsString("R1")}},
			NextToken:           &token,
		}, nil),
		client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackResourceDriftsOutput{
			StackResourceDrifts: []cfntypes.StackResourceDrift{{LogicalResourceId: awsString("R2")}},
		}, nil),
	)

	drifts, err := describeResourceDrifts(context.Background(), client, "vpc")
	require.NoError(t, err)
	require.Len(t, drifts, 2)
	assert.Equal(t, "R1", stringValue(drifts[0].LogicalResourceId))
	assert.Equal(t, "R2", stringValue(drifts[1].LogicalResourceId))
}

// describeResourceDrifts must wrap an API error.
func TestDescribeResourceDrifts_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := describeResourceDrifts(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// runDriftDetect's happy path (no --fail-on-drift): renders the summary line
// and never errors, even when drift was actually found.
func TestRunDriftDetect_Success_NoFailOnDrift(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(&cloudformation.DetectStackDriftOutput{
		StackDriftDetectionId: awsString("detection-1"),
	}, nil)
	count := int32(3)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus:           cfntypes.StackDriftDetectionStatusDetectionComplete,
		StackDriftStatus:          cfntypes.StackDriftStatusDrifted,
		DriftedStackResourceCount: &count,
	}, nil)

	out := captureStdout(t, func() {
		summary, err := runDriftDetect(context.Background(), client, "vpc", false, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, string(cfntypes.StackDriftStatusDrifted), summary["drift_status"])
		assert.Equal(t, int32(3), summary["drifted_resource_count"])
	})
	assert.Contains(t, out, "vpc: DRIFTED (3 resource(s) drifted)")
}

// runDriftDetect must return ErrAwsCloudFormationDriftDetected when
// --fail-on-drift is set and the stack is drifted.
func TestRunDriftDetect_FailOnDrift(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(&cloudformation.DetectStackDriftOutput{
		StackDriftDetectionId: awsString("detection-1"),
	}, nil)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus:  cfntypes.StackDriftDetectionStatusDetectionComplete,
		StackDriftStatus: cfntypes.StackDriftStatusDrifted,
	}, nil)

	_, err := runDriftDetect(context.Background(), client, "vpc", true, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationDriftDetected)
}

// runDriftDetect must NOT fail even with --fail-on-drift set when the stack
// is in fact IN_SYNC — the negative-path complement to
// TestRunDriftDetect_FailOnDrift, proving the failure trigger is gated on the
// drift status, not merely on the flag being set.
func TestRunDriftDetect_FailOnDrift_InSyncDoesNotFail(t *testing.T) {
	shrinkDriftTiming(t, time.Millisecond, time.Minute)

	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(&cloudformation.DetectStackDriftOutput{
		StackDriftDetectionId: awsString("detection-1"),
	}, nil)
	client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackDriftDetectionStatusOutput{
		DetectionStatus:  cfntypes.StackDriftDetectionStatusDetectionComplete,
		StackDriftStatus: cfntypes.StackDriftStatusInSync,
	}, nil)

	_, err := runDriftDetect(context.Background(), client, "vpc", true, map[string]any{})
	require.NoError(t, err)
}

// runDriftDetect must propagate a detectDrift failure.
func TestRunDriftDetect_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	_, err := runDriftDetect(context.Background(), client, "vpc", false, map[string]any{})
	require.Error(t, err)
}

// runDriftDescribe must render a "no drift results" line when the stack has
// none.
func TestRunDriftDescribe_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackResourceDriftsOutput{}, nil)

	out := captureStdout(t, func() {
		summary, err := runDriftDescribe(context.Background(), client, "vpc", map[string]any{})
		require.NoError(t, err)
		assert.Empty(t, summary["drifts"])
	})
	assert.Contains(t, out, "vpc: no drift results")
}

// runDriftDescribe must skip IN_SYNC resources from the rendered output while
// still including them in the summary map, and must print every non-IN_SYNC
// resource. Asserted by presence/absence, not just a non-error return — per
// the no-coverage-theater rule for this exact filter.
func TestRunDriftDescribe_SkipsInSyncResources(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackResourceDriftsOutput{
		StackResourceDrifts: []cfntypes.StackResourceDrift{
			{
				StackResourceDriftStatus: cfntypes.StackResourceDriftStatusInSync,
				ResourceType:             awsString("AWS::S3::Bucket"),
				LogicalResourceId:        awsString("InSyncBucket"),
			},
			{
				StackResourceDriftStatus: cfntypes.StackResourceDriftStatusModified,
				ResourceType:             awsString("AWS::IAM::Role"),
				LogicalResourceId:        awsString("ModifiedRole"),
			},
		},
	}, nil)

	out := captureStdout(t, func() {
		summary, err := runDriftDescribe(context.Background(), client, "vpc", map[string]any{})
		require.NoError(t, err)
		// The summary keeps every drift result, including IN_SYNC ones — only the
		// rendered text output filters them.
		assert.Len(t, summary["drifts"].([]cfntypes.StackResourceDrift), 2)
	})
	assert.NotContains(t, out, "InSyncBucket", "an IN_SYNC resource must be filtered from the rendered output")
	assert.Contains(t, out, "ModifiedRole", "a non-IN_SYNC resource must still be rendered")
}

// runDriftDescribe must propagate a describeResourceDrifts failure.
func TestRunDriftDescribe_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStackResourceDrifts(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := runDriftDescribe(context.Background(), client, "vpc", map[string]any{})
	require.Error(t, err)
}
