package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestRunDiff_Cleanup verifies preview ownership, failure propagation, and cancellation recovery.
func TestRunDiff_Cleanup(t *testing.T) {
	pollErr := errors.New("describe changeset failed")
	cleanupErr := errors.New("cleanup denied")
	cases := []struct {
		name                                                    string
		before, after                                           cfntypes.StackStatus
		pollErr, deleteChangeSetErr, inspectErr, deleteStackErr error
		cancel, noOp                                            bool
	}{
		{name: "new stack", after: cfntypes.StackStatusReviewInProgress},
		{name: "existing stack", before: cfntypes.StackStatusCreateComplete},
		{name: "preexisting empty stack", before: cfntypes.StackStatusReviewInProgress},
		{name: "new stack subsequently deployed", after: cfntypes.StackStatusCreateInProgress},
		{name: "stub already gone"},
		{name: "no changes", before: cfntypes.StackStatusCreateComplete, noOp: true},
		{name: "first poll fails", pollErr: pollErr, after: cfntypes.StackStatusReviewInProgress},
		{name: "cancelled poll", pollErr: context.Canceled, cancel: true, after: cfntypes.StackStatusReviewInProgress},
		{name: "changeset cleanup fails", before: cfntypes.StackStatusCreateComplete, deleteChangeSetErr: cleanupErr},
		{name: "stub inspection fails", inspectErr: cleanupErr},
		{name: "stub deletion fails", after: cfntypes.StackStatusReviewInProgress, deleteStackErr: cleanupErr},
		{name: "poll and cleanup fail", before: cfntypes.StackStatusCreateComplete, pollErr: pollErr, deleteChangeSetErr: cleanupErr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := &cloudformation.DescribeStacksOutput{}
			if tc.before != "" {
				before.Stacks = []cfntypes.Stack{{StackStatus: tc.before}}
			}
			describe := client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(before, nil)
			var createdName string
			create := client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).After(describe).DoAndReturn(
				func(_ context.Context, in *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
					createdName = stringValue(in.ChangeSetName)
					return &cloudformation.CreateChangeSetOutput{Id: awsString("preview-id"), StackId: awsString("stack-id")}, nil
				},
			)
			poll := client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).After(create).DoAndReturn(
				func(_ context.Context, _ *cloudformation.DescribeChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
					if tc.cancel {
						cancel()
					}
					if tc.pollErr != nil {
						return nil, tc.pollErr
					}
					result := &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete, ChangeSetId: awsString("preview-id"), StackId: awsString("stack-id")}
					if tc.noOp {
						result.Status = cfntypes.ChangeSetStatusFailed
						result.StatusReason = awsString("No updates are to be performed")
					}
					return result, nil
				},
			)
			cleanup := client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).After(poll).DoAndReturn(
				func(cleanupCtx context.Context, in *cloudformation.DeleteChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteChangeSetOutput, error) {
					require.NoError(t, cleanupCtx.Err())
					_, bounded := cleanupCtx.Deadline()
					assert.True(t, bounded, "cleanup must have a bounded lifetime")
					assert.Equal(t, createdName, stringValue(in.ChangeSetName))
					assert.Equal(t, "vpc", stringValue(in.StackName))
					return &cloudformation.DeleteChangeSetOutput{}, tc.deleteChangeSetErr
				},
			)
			if tc.before == "" {
				after := &cloudformation.DescribeStacksOutput{}
				if tc.after != "" {
					after.Stacks = []cfntypes.Stack{{StackStatus: tc.after}}
				}
				inspect := client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("stack-id")}).After(cleanup).Return(after, tc.inspectErr)
				if tc.after == cfntypes.StackStatusReviewInProgress && tc.inspectErr == nil {
					expectOwnedPreviewStubDeletion(t, client, inspect, tc.deleteStackErr)
				}
			}
			summary, err := runDiff(&opContext{Ctx: ctx}, client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{})
			expected := errors.Join(tc.pollErr, tc.deleteChangeSetErr, tc.inspectErr, tc.deleteStackErr)
			if expected == nil {
				require.NoError(t, err)
				assert.Equal(t, "preview-id", summary["changeset_id"])
				assert.Equal(t, tc.noOp, summary["no_op"])
			} else {
				require.Error(t, err)
				for _, cause := range []error{tc.pollErr, tc.deleteChangeSetErr, tc.inspectErr, tc.deleteStackErr} {
					if cause != nil {
						assert.ErrorIs(t, err, cause)
					}
				}
			}
		})
	}
}

// TestRunDiff_PreservesOriginalError keeps diagnostic metadata accessible when no cleanup failed.
func TestRunDiff_PreservesOriginalError(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	original := errors.New("stack lookup unavailable")
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, original)
	_, err := runDiff(&opContext{Ctx: context.Background()}, client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{})
	require.Same(t, original, err)
}

// expectOwnedPreviewStubDeletion verifies that an owned stub is deleted and cleanup waits independently of cancellation.
func expectOwnedPreviewStubDeletion(t *testing.T, client *MockCloudFormationClient, inspected *gomock.Call, deleteErr error) {
	t.Helper()
	deleted := client.EXPECT().DeleteStack(gomock.Any(), &cloudformation.DeleteStackInput{StackName: awsString("stack-id")}).After(inspected).Return(&cloudformation.DeleteStackOutput{}, deleteErr)
	if deleteErr != nil {
		return
	}
	client.EXPECT().DescribeStacks(gomock.Any(), &cloudformation.DescribeStacksInput{StackName: awsString("stack-id")}).After(deleted).DoAndReturn(
		func(cleanupCtx context.Context, _ *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
			require.NoError(t, cleanupCtx.Err(), "cancelled previews still wait for cleanup")
			return stackStatusOutput(cfntypes.StackStatusDeleteComplete), nil
		},
	)
}
