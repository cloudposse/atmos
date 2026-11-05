package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Cleanup waits for creation to finish even when the preview's context is cancelled.
func TestDiscardChangeSet_RetriesAfterCreation(t *testing.T) {
	for _, status := range []cfntypes.ChangeSetStatus{cfntypes.ChangeSetStatusCreateComplete, cfntypes.ChangeSetStatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := NewMockCloudFormationClient(gomock.NewController(t))
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				deleted := expectChangeSetCleanupRetry(t, client, status)
				inspected := client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("stack-id")}).After(deleted).
					Return(stackStatusOutput(cfntypes.StackStatusReviewInProgress), nil)
				expectOwnedPreviewStubDeletion(t, client, inspected, nil)
				result := &changeSetResult{ChangeSetName: "preview", StackID: "stack-id", StackStub: true}
				require.NoError(t, discardChangeSet(ctx, client, "vpc", result))
			})
		})
	}
}

// The strict call order rejects retries before a deletable status and premature stub removal.
func expectChangeSetCleanupRetry(t *testing.T, client *MockCloudFormationClient, status cfntypes.ChangeSetStatus) *gomock.Call {
	t.Helper()
	input := &cloudformation.DeleteChangeSetInput{StackName: awsString("vpc"), ChangeSetName: awsString("preview")}
	invalid := &smithy.GenericAPIError{Code: "InvalidChangeSetStatus", Message: "creation is in progress"}
	previous := client.EXPECT().DeleteChangeSet(gomock.Any(), input).DoAndReturn(
		func(ctx context.Context, _ *cloudformation.DeleteChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteChangeSetOutput, error) {
			require.NoError(t, ctx.Err())
			deadline, bounded := ctx.Deadline()
			require.True(t, bounded)
			assert.Equal(t, changeSetCleanupTimeout, time.Until(deadline))
			return nil, fmt.Errorf("delete rejected: %w", invalid)
		},
	)
	for _, current := range []cfntypes.ChangeSetStatus{cfntypes.ChangeSetStatusCreatePending, cfntypes.ChangeSetStatusCreateInProgress, status} {
		previous = client.EXPECT().DescribeChangeSet(gomock.Any(), &cloudformation.DescribeChangeSetInput{
			StackName: awsString("vpc"), ChangeSetName: awsString("preview"),
		}).After(previous).Return(&cloudformation.DescribeChangeSetOutput{Status: current}, nil)
	}
	return client.EXPECT().DeleteChangeSet(gomock.Any(), input).After(previous).Return(&cloudformation.DeleteChangeSetOutput{}, nil)
}

func TestDiscardChangeSet_TerminalCleanupErrors(t *testing.T) {
	denied := errors.New("access denied")
	invalid := &smithy.GenericAPIError{Code: "InvalidChangeSetStatus"}
	cases := []struct {
		name        string
		deleteErr   error
		describeErr error
		status      cfntypes.ChangeSetStatus
	}{
		{name: "delete denied", deleteErr: denied},
		{name: "describe denied", deleteErr: invalid, describeErr: denied},
		{name: "unexpected terminal status", deleteErr: invalid, status: cfntypes.ChangeSetStatusDeleteFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			deleted := client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(nil, tc.deleteErr)
			if errors.Is(tc.deleteErr, invalid) {
				client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).After(deleted).
					Return(&cloudformation.DescribeChangeSetOutput{Status: tc.status}, tc.describeErr)
			}
			err := discardChangeSet(t.Context(), client, "vpc", &changeSetResult{ChangeSetName: "preview", StackStub: true})
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
			assert.ErrorIs(t, err, tc.deleteErr)
			if tc.describeErr != nil {
				assert.ErrorIs(t, err, tc.describeErr)
			}
		})
	}
}

// A stuck creation and repeated status rejections both exhaust the same bounded lifetime.
func TestDiscardChangeSet_RetryDeadline(t *testing.T) {
	for _, status := range []cfntypes.ChangeSetStatus{cfntypes.ChangeSetStatusCreateInProgress, cfntypes.ChangeSetStatusCreateComplete} {
		t.Run(string(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				client := NewMockCloudFormationClient(gomock.NewController(t))
				invalid := &smithy.GenericAPIError{Code: "InvalidChangeSetStatus"}
				deletes := client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(nil, invalid)
				if status == cfntypes.ChangeSetStatusCreateComplete {
					deletes.MinTimes(2)
				}
				client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).
					Return(&cloudformation.DescribeChangeSetOutput{Status: status}, nil).MinTimes(2)
				start := time.Now()
				err := discardChangeSet(t.Context(), client, "vpc", &changeSetResult{ChangeSetName: "preview", StackStub: true})
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.ErrorIs(t, err, invalid)
				assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
				assert.Equal(t, changeSetCleanupTimeout, time.Since(start))
			})
		})
	}
}

func TestDeleteChangeSetWhenReady_Cancelled(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, deleteChangeSetWhenReady(ctx, client, "vpc", "preview"), context.Canceled)
}
