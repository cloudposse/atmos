package cloudformation

import (
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Deleting a stack that does not exist (with or without --retain-resources or
// --disable-termination-protection) is an idempotent success that says so, the
// same way with or without a terminal, instead of printing nothing (non-TTY) or
// a misleading DELETE_COMPLETE (TTY). No event stream is started.
func TestRunDelete_MissingStackIsIdempotent(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]any
	}{
		{name: "plain", flags: nil},
		{name: "retain-resources", flags: map[string]any{"retain-resources": []string{"Bucket"}}},
		{name: "disable-termination-protection", flags: map[string]any{"disable-termination-protection": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}
			gomock.InOrder(
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, missing),
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, missing),
				// No DeleteStack and no event stream: a call fails the test.
			)
			var summary map[string]any
			var err error
			out := captureStderr(t, func() {
				summary, err = runDelete(t.Context(), client, tt.flags, &stackSpec{StackName: "vpc"}, map[string]any{})
			})
			require.NoError(t, err)
			assert.Equal(t, true, summary["already_deleted"])
			assert.NotContains(t, summary, "final_status")
			assert.Contains(t, normalizeUIOutput(out), "vpc does not exist; nothing to delete")
			assert.NotContains(t, out, "DELETE_COMPLETE")
		})
	}
}

// A completed delete reports success on the UI channel.
func TestRunDelete_SuccessReportsDeletedStack(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{Stacks: []cfntypes.Stack{{}}}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&sdk.DeleteStackOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{}, nil),
	)
	var summary map[string]any
	var err error
	out := captureStderr(t, func() {
		summary, err = runDelete(t.Context(), client, nil, &stackSpec{StackName: "vpc"}, map[string]any{})
	})
	require.NoError(t, err)
	assert.Equal(t, string(cfntypes.StackStatusDeleteComplete), summary["final_status"])
	assert.NotContains(t, summary, "already_deleted")
	assert.Contains(t, normalizeUIOutput(out), "Deleted stack vpc")
}

// A delete that ends DELETE_FAILED names the failed logical IDs in a hint that
// suggests --retain-resources.
func TestRunDelete_DeleteFailedHintsRetainResources(t *testing.T) {
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })

	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{Stacks: []cfntypes.Stack{{}}}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&sdk.DeleteStackOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteInProgress}}}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteFailed}}}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStackEventsOutput{}, nil),
		client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(&sdk.ListStackResourcesOutput{
			StackResourceSummaries: []cfntypes.StackResourceSummary{
				{LogicalResourceId: awsString("Queue"), ResourceStatus: cfntypes.ResourceStatusDeleteComplete},
				{LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusDeleteFailed},
				{LogicalResourceId: awsString("Table"), ResourceStatus: cfntypes.ResourceStatusDeleteFailed},
			},
		}, nil),
	)
	_, err := runDelete(t.Context(), client, nil, &stackSpec{StackName: "vpc"}, map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "--retain-resources=Bucket,Table")
}

// Negative path: a delete that fails in any other status gets no
// --retain-resources hint, and the resources are never listed.
func TestDeleteFailedError_OnlyDeleteFailedGetsRetainHint(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t)) // no ListStackResources expectation.
	err := deleteFailedError(t.Context(), client, "vpc", cfntypes.StackStatusRollbackComplete)
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
	assert.Empty(t, cockroachErrors.GetAllHints(err))
}

// A failure to list the failed resources only drops the IDs from the hint.
func TestDeleteFailedError_ListFailureStillHints(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))
	err := deleteFailedError(t.Context(), client, "vpc", cfntypes.StackStatusDeleteFailed)
	hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
	assert.Contains(t, hints, "--retain-resources=<LogicalResourceId>")
}

func TestDeleteStack_PreservesOtherAWSErrors(t *testing.T) {
	for _, apiErr := range []*smithy.GenericAPIError{
		{Code: "AccessDenied", Message: "Stack with id vpc does not exist or access is denied"},
		{Code: "ValidationError", Message: "Invalid stack name"},
	} {
		t.Run(apiErr.Code, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return((*sdk.DescribeStacksOutput)(nil), apiErr)
			_, err := deleteStack(t.Context(), client, &stackSpec{StackName: "vpc"}, deleteOptions{})
			require.ErrorIs(t, err, apiErr)
		})
	}
}

func TestDeleteStack_DisappearsAfterProtectionDisabled(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: awsBool(true)}},
		}, nil),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&sdk.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, missing),
	)
	alreadyGone, err := deleteStack(t.Context(), client, &stackSpec{StackName: "vpc"}, deleteOptions{DisableTerminationProtection: true})
	require.NoError(t, err)
	// The stack vanished while DeleteStack ran: the delete proceeds to event streaming.
	require.False(t, alreadyGone)
}

// With termination_protection: true in local config and the stack already gone,
// delete must report "nothing to delete" instead of failing the protection
// gate: there is no stack left to protect.
func TestDeleteStack_MissingStackBeatsLocalTerminationProtection(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}
	// No DeleteStack or UpdateTerminationProtection expectation: a call fails the test.
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, missing)

	alreadyGone, err := deleteStack(t.Context(), client, &stackSpec{StackName: "vpc", TerminationProtection: true}, deleteOptions{})
	require.NoError(t, err)
	assert.True(t, alreadyGone)
}

// Negative path: a stack that exists with local termination_protection: true
// still hits the gate, and a live-protected stack with drifted local config
// does too.
func TestDeleteStack_ExistingProtectedStackStillBlocked(t *testing.T) {
	tests := []struct {
		name  string
		local bool
		live  bool
	}{
		{name: "local and live protected", local: true, live: true},
		{name: "local protected, live drifted off", local: true, live: false},
		{name: "local off, live protected", local: false, live: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{
				Stacks: []cfntypes.Stack{{EnableTerminationProtection: awsBool(tt.live)}},
			}, nil)

			alreadyGone, err := deleteStack(t.Context(), client, &stackSpec{StackName: "vpc", TerminationProtection: tt.local}, deleteOptions{})
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationTerminationProtectionEnabled)
			assert.False(t, alreadyGone)
		})
	}
}
