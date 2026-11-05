package cloudformation

import (
	"context"
	"errors"
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

func TestRemoveStubStack_WaitsForDeletion(t *testing.T) {
	originalInterval := eventPollInterval
	eventPollInterval = time.Nanosecond
	t.Cleanup(func() { eventPollInterval = originalInterval })
	denied := errors.New("access denied")
	cases := []struct {
		name    string
		status  cfntypes.StackStatus
		pollErr error
		wantErr error
		cancel  bool
	}{
		{name: "deleted record", status: cfntypes.StackStatusDeleteComplete},
		{name: "empty response"},
		{name: "missing stack", pollErr: &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id stack-id does not exist"}},
		{name: "deletion failed", status: cfntypes.StackStatusDeleteFailed, wantErr: errUtils.ErrAwsCloudFormationChangeSetFailed},
		{name: "describe denied", pollErr: denied, wantErr: denied},
		{name: "cancelled wait", status: cfntypes.StackStatusDeleteInProgress, cancel: true, wantErr: context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := &cloudformation.DescribeStacksInput{StackName: awsString("stack-id")}
			inspect := client.EXPECT().DescribeStacks(gomock.Any(), input).Return(stackStatusOutput(cfntypes.StackStatusReviewInProgress), nil)
			deletion := client.EXPECT().DeleteStack(gomock.Any(), &cloudformation.DeleteStackInput{StackName: awsString("stack-id")}).After(inspect).Return(&cloudformation.DeleteStackOutput{}, nil)
			pending := client.EXPECT().DescribeStacks(gomock.Any(), input).After(deletion).Return(stackStatusOutput(cfntypes.StackStatusDeleteInProgress), nil)
			client.EXPECT().DescribeStacks(gomock.Any(), input).After(pending).DoAndReturn(
				func(_ context.Context, _ *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
					if tc.cancel {
						cancel()
					}
					return stackStatusOutput(tc.status), tc.pollErr
				},
			)
			err := removeStubStack(ctx, client, "stack-id")
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Contains(t, err.Error(), "stack-id")
			}
		})
	}
}

func stackStatusOutput(status cfntypes.StackStatus) *cloudformation.DescribeStacksOutput {
	output := &cloudformation.DescribeStacksOutput{}
	if status != "" {
		output.Stacks = []cfntypes.Stack{{StackStatus: status}}
	}
	return output
}

func TestWaitForStubDeletion_Deadline(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	require.ErrorIs(t, waitForStubDeletion(ctx, client, "stack-id"), context.DeadlineExceeded)
}

// The virtual clock verifies a stuck deletion hits its deadline without a real sleep.
func TestRemoveStubStack_DeletionTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := NewMockCloudFormationClient(gomock.NewController(t))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		input := &cloudformation.DescribeStacksInput{StackName: awsString("stack-id")}
		inspect := client.EXPECT().DescribeStacks(gomock.Any(), input).Return(stackStatusOutput(cfntypes.StackStatusReviewInProgress), nil)
		deleted := client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).After(inspect).Return(&cloudformation.DeleteStackOutput{}, nil)
		client.EXPECT().DescribeStacks(gomock.Any(), input).After(deleted).Return(stackStatusOutput(cfntypes.StackStatusDeleteInProgress), nil)
		err := removeStubStack(ctx, client, "stack-id")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "stack-id")
	})
}
