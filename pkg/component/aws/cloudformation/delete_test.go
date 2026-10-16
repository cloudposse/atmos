package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDeleteStack_BlocksOnTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	// No API calls expected — the guard must short-circuit before calling DeleteStack.

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

func TestDeleteStack_DisableTerminationProtectionFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.NoError(t, err)
}

// deleteStack must restore termination protection when DeleteStack fails
// after protection was disabled via --disable-termination-protection --
// otherwise a failed delete attempt silently leaves a previously-protected
// stack unprotected.
func TestDeleteStack_RestoresTerminationProtectionOnDeleteFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), &cloudformation.UpdateTerminationProtectionInput{
			StackName:                   awsString("vpc"),
			EnableTerminationProtection: awsBool(false),
		}).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, errors.New("delete rejected")),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), &cloudformation.UpdateTerminationProtectionInput{
			StackName:                   awsString("vpc"),
			EnableTerminationProtection: awsBool(true),
		}).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.Contains(t, err.Error(), "delete rejected")
}

// deleteStack must join a restoration failure with the original delete error
// (rather than swallowing either) when restoring termination protection also
// fails for a reason unrelated to the stack having actually entered deletion.
func TestDeleteStack_RestoreFailureJoinedWithDeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, errors.New("delete rejected")),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied")),
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete rejected", "must preserve the original delete error")
	assert.Contains(t, err.Error(), "access denied", "must include the restoration failure with context")
}

// deleteStack must not treat AWS's DELETE_IN_PROGRESS/DELETE_COMPLETE
// rejection of the restore attempt as a failure: the delete actually started
// server-side despite the observed DeleteStack error, so there is nothing to
// restore and only the original delete error should surface.
func TestDeleteStack_RestoreSkippedWhenDeleteActuallyInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, errors.New("timeout")),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).
			Return(nil, &smithy.GenericAPIError{Code: "ValidationError", Message: "cannot update termination protection while stack is DELETE_IN_PROGRESS"}),
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.NotContains(t, err.Error(), "DELETE_IN_PROGRESS",
		"the expected DELETE_IN_PROGRESS restoration rejection must not be surfaced as an additional error")
}

// Restoration failures must retain their identity even when unrelated error text
// mentions a delete state; only a typed AWS validation rejection can be ignored.
func TestRestoreTerminationProtection_DeleteStateErrorClassification(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		ignored bool
	}{
		{name: "success", ignored: true},
		{name: "delete in progress", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "stack is DELETE_IN_PROGRESS"}, ignored: true},
		{name: "delete complete", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "stack is DELETE_COMPLETE"}, ignored: true},
		{name: "wrapped validation", err: fmt.Errorf("update: %w", &smithy.GenericAPIError{Code: "ValidationError", Message: "stack is DELETE_IN_PROGRESS"}), ignored: true},
		{name: "untyped delete state", err: errors.New("transport failure for DELETE_IN_PROGRESS")},
		{name: "access denied", err: &smithy.GenericAPIError{Code: "AccessDenied", Message: "cannot access DELETE_COMPLETE"}},
		{name: "unrelated validation", err: &smithy.GenericAPIError{Code: "ValidationError", Message: "invalid stack name"}},
		{name: "wrapper only", err: fmt.Errorf("DELETE_IN_PROGRESS: %w", &smithy.GenericAPIError{Code: "ValidationError", Message: "invalid stack name"})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().UpdateTerminationProtection(gomock.Any(), &cloudformation.UpdateTerminationProtectionInput{
				StackName: awsString("vpc"), EnableTerminationProtection: awsBool(true),
			}).Return(nil, tc.err)
			err := restoreTerminationProtectionAfterFailedDelete(context.Background(), client, "vpc")
			if tc.ignored {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.err)
			assert.Contains(t, err.Error(), "restoring termination protection")
		})
	}
}

func TestDeleteStack_NoTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.NoError(t, err)
}

func TestDeleteStack_RetainResourcesRequiresDeleteFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{RetainResources: []string{"MyBucket"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// TestDeleteStack_RetainResourcesRejectionLeavesTerminationProtectionUntouched
// guards the guard ordering in deleteStack: when both
// --disable-termination-protection and --retain-resources are passed together
// against a stack that is NOT in DELETE_FAILED status, guardRetainResources
// must reject the request before guardTerminationProtection gets a chance to
// disable termination protection. If the guards ran in the opposite order,
// termination protection would already be disabled with no DeleteStack call
// to trigger the restore path in handleDeleteStackError, silently leaving a
// previously-protected stack unprotected.
func TestDeleteStack_RetainResourcesRejectionLeavesTerminationProtectionUntouched(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// UpdateTerminationProtection and DeleteStack must never be called.
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
	}, nil)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{
		RetainResources:              []string{"MyBucket"},
		DisableTerminationProtection: true,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

func TestDeleteStack_RetainResourcesAllowedWhenDeleteFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteFailed}},
	}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.DeleteStackInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error) {
			assert.Equal(t, []string{"MyBucket"}, input.RetainResources)
			return &cloudformation.DeleteStackOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{RetainResources: []string{"MyBucket"}})
	require.NoError(t, err)
}

func TestDisableTerminationProtection_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	err := disableTerminationProtection(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// currentStackStatus must wrap a DescribeStacks API error.
func TestCurrentStackStatus_DescribeStacksError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := currentStackStatus(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// currentStackStatus must error (not panic) when DescribeStacks returns no
// matching stack.
func TestCurrentStackStatus_NoStackFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	_, err := currentStackStatus(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vpc")
}

func TestIsDeleteFailedStack(t *testing.T) {
	assert.True(t, isDeleteFailedStack(cfntypes.StackStatusDeleteFailed))
	assert.False(t, isDeleteFailedStack(cfntypes.StackStatusUpdateComplete))
}
