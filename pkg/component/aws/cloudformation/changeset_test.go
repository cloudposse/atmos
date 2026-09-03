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

func TestStackExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(m *MockCloudFormationClient)
		expected bool
		wantErr  bool
	}{
		{
			name: "existing stack",
			setup: func(m *MockCloudFormationClient) {
				m.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
					Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}},
				}, nil)
			},
			expected: true,
		},
		{
			name: "not found",
			setup: func(m *MockCloudFormationClient) {
				m.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("stack vpc does not exist"))
			},
			expected: false,
		},
		{
			name: "review-in-progress treated as not existing",
			setup: func(m *MockCloudFormationClient) {
				m.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
					Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusReviewInProgress}},
				}, nil)
			},
			expected: false,
		},
		{
			name: "other API error propagates",
			setup: func(m *MockCloudFormationClient) {
				m.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)
			tt.setup(client)

			exists, err := stackExists(context.Background(), client, "vpc")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, exists)
		})
	}
}

func TestEvaluateChangeSetStatus(t *testing.T) {
	tests := []struct {
		name         string
		status       cfntypes.ChangeSetStatus
		statusReason string
		wantDecision changeSetPollDecision
		wantNoOp     bool
		wantErr      bool
	}{
		{
			name:         "create complete",
			status:       cfntypes.ChangeSetStatusCreateComplete,
			wantDecision: changeSetPollDone,
		},
		{
			name:         "no-op failure",
			status:       cfntypes.ChangeSetStatusFailed,
			statusReason: "The submitted information didn't contain changes.",
			wantDecision: changeSetPollDone,
			wantNoOp:     true,
		},
		{
			name:         "real failure",
			status:       cfntypes.ChangeSetStatusFailed,
			statusReason: "Template format error",
			wantDecision: changeSetPollError,
			wantErr:      true,
		},
		{
			name:         "still in progress",
			status:       cfntypes.ChangeSetStatusCreateInProgress,
			wantDecision: changeSetPollContinue,
		},
		{
			name:         "unexpected terminal status",
			status:       cfntypes.ChangeSetStatus("SOMETHING_WEIRD"),
			wantDecision: changeSetPollError,
			wantErr:      true,
		},
		{
			// UPDATE_IN_PROGRESS (unlike CREATE_IN_PROGRESS/CREATE_PENDING) has no
			// dedicated enum case above — it falls through to the suffix-based
			// default branch, which must still classify it as "keep polling".
			name:         "update in progress falls through to suffix match",
			status:       cfntypes.ChangeSetStatus("UPDATE_IN_PROGRESS"),
			wantDecision: changeSetPollContinue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &changeSetResult{Status: tt.status, StatusReason: tt.statusReason}
			decision, err := evaluateChangeSetStatus(result)
			assert.Equal(t, tt.wantDecision, decision)
			assert.Equal(t, tt.wantNoOp, result.NoOp)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCreateChangeSet_DetectsCreateVsUpdate(t *testing.T) {
	tests := []struct {
		name            string
		stackExists     bool
		wantChangeSetTy cfntypes.ChangeSetType
	}{
		{name: "new stack uses CREATE", stackExists: false, wantChangeSetTy: cfntypes.ChangeSetTypeCreate},
		{name: "existing stack uses UPDATE", stackExists: true, wantChangeSetTy: cfntypes.ChangeSetTypeUpdate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewMockCloudFormationClient(ctrl)

			describeStacksOut := &cloudformation.DescribeStacksOutput{}
			if tt.stackExists {
				describeStacksOut.Stacks = []cfntypes.Stack{{StackStatus: cfntypes.StackStatusCreateComplete}}
			}
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(describeStacksOut, nil)

			var gotChangeSetType cfntypes.ChangeSetType
			client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
					gotChangeSetType = input.ChangeSetType
					return &cloudformation.CreateChangeSetOutput{}, nil
				},
			)

			client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
				Status: cfntypes.ChangeSetStatusCreateComplete,
			}, nil)

			spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
			result, err := createChangeSet(context.Background(), client, spec)
			require.NoError(t, err)
			assert.Equal(t, tt.wantChangeSetTy, gotChangeSetType)
			assert.False(t, result.NoOp)
		})
	}
}

func TestExecuteChangeSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	result := &changeSetResult{ChangeSetName: "atmos-vpc-123"}
	err := executeChangeSet(context.Background(), client, spec, result)
	require.NoError(t, err)
}

// executeChangeSet must not set DisableRollback on ExecuteChangeSet when
// OnStackFailure was already set on the CREATE changeset (spec.DisableRollback
// with a new stack) — AWS's API rejects a changeset execution that specifies
// both, so setting both unconditionally would make disable_rollback: true
// always fail on stack creation.
func TestExecuteChangeSet_CreateWithDisableRollback_OmitsDisableRollback(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.ExecuteChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error) {
			assert.Nil(t, input.DisableRollback, "DisableRollback must be omitted when OnStackFailure was set on the CREATE changeset")
			return &cloudformation.ExecuteChangeSetOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc", DisableRollback: true}
	result := &changeSetResult{ChangeSetName: "atmos-vpc-123", ChangeSetType: cfntypes.ChangeSetTypeCreate}
	err := executeChangeSet(context.Background(), client, spec, result)
	require.NoError(t, err)
}

// executeChangeSet must still set DisableRollback for an UPDATE changeset —
// OnStackFailure is a CreateChangeSet-only, CREATE-only parameter, so no
// conflict exists there.
func TestExecuteChangeSet_UpdateWithDisableRollback_SetsDisableRollback(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.ExecuteChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error) {
			require.NotNil(t, input.DisableRollback)
			assert.True(t, *input.DisableRollback)
			return &cloudformation.ExecuteChangeSetOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc", DisableRollback: true}
	result := &changeSetResult{ChangeSetName: "atmos-vpc-123", ChangeSetType: cfntypes.ChangeSetTypeUpdate}
	err := executeChangeSet(context.Background(), client, spec, result)
	require.NoError(t, err)
}

func TestExecuteChangeSet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	spec := &stackSpec{StackName: "vpc"}
	result := &changeSetResult{ChangeSetName: "atmos-vpc-123"}
	err := executeChangeSet(context.Background(), client, spec, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// waitForChangeSet must wrap a DescribeChangeSet API error with
// ErrAwsCloudFormationChangeSetFailed rather than looping or panicking.
func TestWaitForChangeSet_DescribeChangeSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := waitForChangeSet(context.Background(), client, "vpc", "atmos-vpc-123", cfntypes.ChangeSetTypeCreate)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// waitForChangeSet must follow DescribeChangeSet's NextToken and collect every
// page's Changes — a changeset with enough resource changes to paginate would
// otherwise silently under-report the diff a user reviews before approving apply.
func TestWaitForChangeSet_FollowsPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	page1Change := cfntypes.Change{Type: cfntypes.ChangeTypeResource}
	page2Change := cfntypes.Change{Type: cfntypes.ChangeTypeResource}
	nextToken := "page-2-token"

	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.DescribeChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
				assert.Nil(t, input.NextToken)
				return &cloudformation.DescribeChangeSetOutput{
					Status:    cfntypes.ChangeSetStatusCreateComplete,
					Changes:   []cfntypes.Change{page1Change},
					NextToken: &nextToken,
				}, nil
			},
		),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.DescribeChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
				require.NotNil(t, input.NextToken)
				assert.Equal(t, nextToken, *input.NextToken)
				return &cloudformation.DescribeChangeSetOutput{
					Status:  cfntypes.ChangeSetStatusCreateComplete,
					Changes: []cfntypes.Change{page2Change},
				}, nil
			},
		),
	)

	result, err := waitForChangeSet(context.Background(), client, "vpc", "atmos-vpc-123", cfntypes.ChangeSetTypeCreate)
	require.NoError(t, err)
	assert.Equal(t, []cfntypes.Change{page1Change, page2Change}, result.Changes)
}

func TestSanitizeChangeSetSuffix(t *testing.T) {
	assert.Equal(t, "acme-plat-ue2-dev-vpc", sanitizeChangeSetSuffix("acme-plat-ue2-dev-vpc"))
	assert.Equal(t, "acme-plat-vpc", sanitizeChangeSetSuffix("acme_plat-vpc"), "non-alphanumeric-non-hyphen characters (e.g. underscore) become hyphens")
}

// timeValue must return the zero time.Time for a nil pointer and the pointee
// value for a non-nil one.
func TestTimeValue(t *testing.T) {
	assert.True(t, timeValue(nil).IsZero())

	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, ts, timeValue(&ts))
}
