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

func TestSanitizeChangeSetSuffix(t *testing.T) {
	assert.Equal(t, "acme-plat-ue2-dev-vpc", sanitizeChangeSetSuffix("acme-plat-ue2-dev-vpc"))
	assert.Equal(t, "acme-plat-vpc", sanitizeChangeSetSuffix("acme_plat-vpc"), "non-alphanumeric-non-hyphen characters (e.g. underscore) become hyphens")
}
