package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
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
			Return(nil, errors.New("ValidationError: cannot update termination protection while stack is DELETE_IN_PROGRESS")),
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.NotContains(t, err.Error(), "DELETE_IN_PROGRESS",
		"the expected DELETE_IN_PROGRESS restoration rejection must not be surfaced as an additional error")
}

func TestDeleteStack_NoTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// Local config says false, so the live state must be checked before the
	// gate is skipped; live state reports protection off too, so delete
	// proceeds with no hint.
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(false)}},
	}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.NoError(t, err)
}

// TestDeleteStack_BlocksOnLiveTerminationProtection proves the fix for the
// field-test bug: local config says termination_protection: false (drifted
// from AWS because apply only ever turns protection ON, never OFF — see
// applyTerminationProtection), but the stack is still live-protected in AWS.
// The gate must still fire the actionable hint, and DeleteStack must never be
// called.
func TestDeleteStack_BlocksOnLiveTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(true)}},
	}, nil)
	// No DeleteStack expectation — the guard must short-circuit before it.

	spec := &stackSpec{StackName: "vpc", TerminationProtection: false}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// TestDeleteStack_DisableTerminationProtectionFlag_SkipsLiveLookup proves
// --disable-termination-protection unconditionally disables protection (via
// UpdateTerminationProtection) and proceeds to DeleteStack without ever
// issuing a DescribeStacks lookup first, regardless of which signal (local
// config or live AWS state) would otherwise have reported protection on.
func TestDeleteStack_DisableTerminationProtectionFlag_SkipsLiveLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// No DescribeStacks expectation — must not be called.
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	// Local config says false (the drifted case), yet --disable-termination-protection still works.
	spec := &stackSpec{StackName: "vpc", TerminationProtection: false}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
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

// describeStack must wrap a DescribeStacks API error.
func TestDescribeStack_DescribeStacksError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := describeStack(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
}

// describeStack must error (not panic) when DescribeStacks returns no
// matching stack.
func TestDescribeStack_NoStackFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	_, err := describeStack(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vpc")
}

func TestIsDeleteFailedStack(t *testing.T) {
	assert.True(t, isDeleteFailedStack(cfntypes.StackStatusDeleteFailed))
	assert.False(t, isDeleteFailedStack(cfntypes.StackStatusUpdateComplete))
}
