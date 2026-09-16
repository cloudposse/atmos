package cloudformation

import (
	"context"
	"errors"
	"testing"
	"time"

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

	// --disable-termination-protection now reads the stack's live protection
	// state immediately before disabling it (see
	// disableTerminationProtectionIfNeeded), rather than unconditionally
	// disabling — that live read is what lets a failed DeleteStack later know
	// whether restoring protection would be correct.
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(true)}},
	}, nil)
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
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(true)}},
		}, nil),
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

func TestHandleDeleteStackError_RestoresProtectionWithIndependentDeadline(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "canceled"
		if expired {
			name = "deadline exceeded"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if expired {
				ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				defer cancel()
			}
			require.Error(t, ctx.Err())
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)
			var cleanupCtx context.Context
			client.EXPECT().UpdateTerminationProtection(gomock.Any(), &cloudformation.UpdateTerminationProtectionInput{
				StackName:                   aws.String("vpc"),
				EnableTerminationProtection: aws.Bool(true),
			}).DoAndReturn(func(restoreCtx context.Context, _ *cloudformation.UpdateTerminationProtectionInput, _ ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error) {
				cleanupCtx = restoreCtx
				require.NoError(t, restoreCtx.Err())
				deadline, ok := restoreCtx.Deadline()
				require.True(t, ok, "cleanup must be bounded")
				assert.Positive(t, time.Until(deadline))
				assert.LessOrEqual(t, time.Until(deadline), 30*time.Second)
				return &cloudformation.UpdateTerminationProtectionOutput{}, nil
			})

			err := handleDeleteStackError(ctx, deleteAttempt{
				Client: client, Spec: &stackSpec{StackName: "vpc"}, WasProtected: true,
			}, ctx.Err())
			assert.ErrorIs(t, err, ctx.Err())
			assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
			require.NotNil(t, cleanupCtx)
			assert.ErrorIs(t, cleanupCtx.Err(), context.Canceled, "cleanup must be released after restoration")
		})
	}
}

// deleteStack must NOT restore termination protection after a failed
// DeleteStack when the stack was never actually protected live to begin with
// (e.g. --disable-termination-protection was passed redundantly). Restoring
// unconditionally would turn an originally-unprotected stack into a
// protected one purely as a side effect of a failed delete attempt.
func TestDeleteStack_DoesNotRestoreWhenStackWasNeverProtected(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(false)}},
		}, nil),
		// No UpdateTerminationProtection(false) call — the stack was already unprotected.
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, errors.New("delete rejected")),
		// No UpdateTerminationProtection(true) restoration call either.
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: false}
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	restoreErr := errors.New("access denied")

	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(true)}},
		}, nil),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, *cloudformation.DeleteStackInput, ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error) {
			cancel()
			return nil, ctx.Err()
		}),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).DoAndReturn(func(restoreCtx context.Context, _ *cloudformation.UpdateTerminationProtectionInput, _ ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error) {
			require.NoError(t, restoreCtx.Err())
			return nil, restoreErr
		}),
	)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(ctx, client, spec, deleteOptions{DisableTerminationProtection: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled, "must preserve the original delete error")
	assert.ErrorIs(t, err, restoreErr, "must include the restoration failure")
}

// deleteStack must not treat AWS's DELETE_IN_PROGRESS/DELETE_COMPLETE
// rejection of the restore attempt as a failure: the delete actually started
// server-side despite the observed DeleteStack error, so there is nothing to
// restore and only the original delete error should surface.
func TestDeleteStack_RestoreSkippedWhenDeleteActuallyInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(true)}},
		}, nil),
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

// TestDeleteStack_DisableTerminationProtectionFlag_SkipsUpdateWhenNotProtected
// proves --disable-termination-protection reads the stack's live protection
// state (regardless of local config's possibly-drifted value) and, when the
// stack is not actually protected, skips the UpdateTerminationProtection(false)
// call entirely while still proceeding to DeleteStack. Skipping that call is
// what lets a later failed delete correctly avoid restoring protection on a
// stack that was never protected to begin with (see
// TestDeleteStack_DoesNotRestoreWhenStackWasNeverProtected).
func TestDeleteStack_DisableTerminationProtectionFlag_SkipsUpdateWhenNotProtected(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{EnableTerminationProtection: aws.Bool(false)}},
	}, nil)
	// No UpdateTerminationProtection expectation — must not be called when the stack isn't protected.
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

// TestDeleteStack_RetainResourcesFailureNeverDisablesProtection is a
// regression test: when both --disable-termination-protection and
// --retain-resources are passed together and the stack is NOT in
// DELETE_FAILED status, --retain-resources validation must fail before
// termination protection is ever disabled. Disabling protection first (the
// prior, buggy ordering) would leave the stack unprotected with no
// DeleteStack call -- and therefore no restoration path -- ever having run.
func TestDeleteStack_RetainResourcesFailureNeverDisablesProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			StackStatus:                 cfntypes.StackStatusUpdateComplete,
			EnableTerminationProtection: aws.Bool(true),
		}},
	}, nil)
	// No UpdateTerminationProtection or DeleteStack expectation — both must be
	// unreachable once --retain-resources validation fails.

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{
		DisableTerminationProtection: true,
		RetainResources:              []string{"MyBucket"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// TestDeleteStack_RetainResourcesSuccessReusesDescribedStackForProtectionCheck
// proves that when --retain-resources validation succeeds, its already-fetched
// describedStack is reused by disableTerminationProtectionIfNeeded instead of
// issuing a second DescribeStacks call.
func TestDeleteStack_RetainResourcesSuccessReusesDescribedStackForProtectionCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	// Exactly one DescribeStacks call expected (Times(1) is gomock's default,
	// asserted explicitly here to make the "no second call" contract visible).
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			StackStatus:                 cfntypes.StackStatusDeleteFailed,
			EnableTerminationProtection: aws.Bool(true),
		}},
	}, nil).Times(1)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{
		DisableTerminationProtection: true,
		RetainResources:              []string{"MyBucket"},
	})
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
