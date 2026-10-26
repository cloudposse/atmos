package cloudformation

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Configured operation deadlines must retain the drift error classification even during a stalled SDK request.
func TestDriftConfiguredTimeoutDuringRequest(t *testing.T) {
	for _, initial := range []bool{true, false} {
		t.Run(map[bool]string{true: "start", false: "poll"}[initial], func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			if initial {
				client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, _ *cloudformation.DetectStackDriftInput, _ ...func(*cloudformation.Options)) (*cloudformation.DetectStackDriftOutput, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
				)
			} else {
				client.EXPECT().DetectStackDrift(gomock.Any(), gomock.Any()).Return(&cloudformation.DetectStackDriftOutput{StackDriftDetectionId: awsString("detection-1")}, nil)
				client.EXPECT().DescribeStackDriftDetectionStatus(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, _ *cloudformation.DescribeStackDriftDetectionStatusInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStackDriftDetectionStatusOutput, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
				)
			}
			parent, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := runRemoteOperation(&opContext{Ctx: parent}, OperationDriftDetect, client, &stackSpec{StackName: "demo", Timeout: 10 * time.Millisecond}, nil)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Contains(t, err.Error(), "timed out waiting for drift detection")
			assert.NoError(t, parent.Err())
		})
	}
}

// Poll waits must classify both an already-expired deadline and one that expires while waiting.
func TestDriftConfiguredTimeoutBetweenPolls(t *testing.T) {
	for _, timeout := range []time.Duration{-time.Second, 10 * time.Millisecond} {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		child, stop := withDefaultTimeout(ctx, time.Minute)
		err := waitForNextDriftPoll(ctx, child)
		stop()
		cancel()
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "timed out waiting for drift detection")
	}
}

// Caller cancellation must remain distinguishable from a deadline during requests and poll waits.
func TestDriftCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	child, stop := withDefaultTimeout(ctx, time.Minute)
	defer stop()
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	err := waitForNextDriftPoll(ctx, child)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	err = classifyDriftPollError(ctx, child, context.Canceled)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}
