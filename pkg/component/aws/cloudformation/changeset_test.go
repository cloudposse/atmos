package cloudformation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	cockroachErrors "github.com/cockroachdb/errors"
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

// createChangeSet must send TemplateURL (not TemplateBody) when spec.TemplateURL
// is set (a packaged, over-the-inline-limit template) -- CreateChangeSet
// accepts exactly one of the two and rejects a request setting both.
func TestCreateChangeSet_UsesTemplateURLWhenPackaged(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	var gotInput *cloudformation.CreateChangeSetInput
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
			gotInput = input
			return &cloudformation.CreateChangeSetOutput{}, nil
		},
	)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
	}, nil)

	spec := &stackSpec{
		StackName:    "vpc",
		TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'",
		TemplateURL:  "s3://my-bucket/dev/vpc/template-abc",
	}
	_, err := createChangeSet(context.Background(), client, spec)
	require.NoError(t, err)

	require.NotNil(t, gotInput)
	require.NotNil(t, gotInput.TemplateURL)
	assert.Equal(t, spec.TemplateURL, *gotInput.TemplateURL)
	assert.Nil(t, gotInput.TemplateBody, "TemplateBody must not be set alongside TemplateURL")
}

// createChangeSet must propagate a stackExists failure (an API error other
// than "does not exist") without attempting to create a changeset.
func TestCreateChangeSet_StackExistsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	_, err := createChangeSet(context.Background(), client, spec)
	require.Error(t, err)
}

// createChangeSet must thread RoleArn into RoleARN and set OnStackFailure
// (only) for a CREATE changeset with DisableRollback.
func TestCreateChangeSet_SetsRoleArnAndOnStackFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	var gotInput *cloudformation.CreateChangeSetInput
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.CreateChangeSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
			gotInput = input
			return &cloudformation.CreateChangeSetOutput{}, nil
		},
	)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
	}, nil)

	spec := &stackSpec{
		StackName:       "vpc",
		TemplateBody:    "AWSTemplateFormatVersion: '2010-09-09'",
		RoleArn:         "arn:aws:iam::123456789012:role/cfn-deploy",
		DisableRollback: true,
	}
	_, err := createChangeSet(context.Background(), client, spec)
	require.NoError(t, err)

	require.NotNil(t, gotInput.RoleARN)
	assert.Equal(t, spec.RoleArn, *gotInput.RoleARN)
	assert.Equal(t, cfntypes.OnStackFailureDoNothing, gotInput.OnStackFailure)
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

// waitForChangeSet must propagate a pagination (NextToken-follow) failure,
// still returning the partial result gathered before the failing page.
func TestWaitForChangeSet_PaginationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	page1Change := cfntypes.Change{Type: cfntypes.ChangeTypeResource}
	nextToken := "page-2-token"

	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status:    cfntypes.ChangeSetStatusCreateComplete,
			Changes:   []cfntypes.Change{page1Change},
			NextToken: &nextToken,
		}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled")),
	)

	result, err := waitForChangeSet(context.Background(), client, "vpc", "atmos-vpc-123", cfntypes.ChangeSetTypeCreate)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
	require.NotNil(t, result, "partial result must still be returned alongside the pagination error")
	assert.Equal(t, []cfntypes.Change{page1Change}, result.Changes)
}

// waitForChangeSet must return promptly with the context's error when the
// context is cancelled while still polling (status stuck in-progress).
func TestWaitForChangeSet_ContextCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateInProgress,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel up front so the select's <-ctx.Done() case fires immediately.

	_, err := waitForChangeSet(ctx, client, "vpc", "atmos-vpc-123", cfntypes.ChangeSetTypeCreate)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSanitizeChangeSetSuffix(t *testing.T) {
	assert.Equal(t, "acme-plat-ue2-dev-vpc", sanitizeChangeSetSuffix("acme-plat-ue2-dev-vpc"))
	assert.Equal(t, "acme-plat-vpc", sanitizeChangeSetSuffix("acme_plat-vpc"), "non-alphanumeric-non-hyphen characters (e.g. underscore) become hyphens")
}

// TestChangeSetName_MaxLengthStackName guards against a CloudFormation
// CreateChangeSet rejection: ChangeSetName (like stack names) is capped at 128
// characters, but changeSetName previously concatenated "atmos-<suffix>-<nanos>"
// with no bound, so a maximum-length (128-char) stack name produced a
// ChangeSetName well over the limit and CreateChangeSet would reject the
// deployment outright.
func TestChangeSetName_MaxLengthStackName(t *testing.T) {
	maxStackName := strings.Repeat("a", 128)

	name := changeSetName(maxStackName)

	assert.LessOrEqual(t, len(name), changeSetNameMaxLength,
		"generated change-set name must never exceed CloudFormation's 128-character ChangeSetName limit")
	assert.True(t, strings.HasPrefix(name, "atmos-"), "name should keep the atmos- prefix")
}

// A short, ordinary stack name must still produce a name that round-trips the
// full sanitized suffix (no truncation needed).
func TestChangeSetName_ShortStackName(t *testing.T) {
	name := changeSetName("vpc")

	assert.LessOrEqual(t, len(name), changeSetNameMaxLength)
	assert.Contains(t, name, "atmos-vpc-", "short stack names should not be truncated")
}

// wrapAPICallError must recognize AWS's "does not exist" validation error
// shape (the one a follow-up call like UpdateTerminationProtection/
// SetStackPolicy/DescribeStacks hits when it targets a stack that was never
// actually deployed) and add an explanation + actionable hint via the error
// builder, while still matching both the sentinel and the original AWS error
// via errors.Is.
func TestWrapAPICallError_StackNotFound(t *testing.T) {
	awsErr := errors.New(`operation error CloudFormation: UpdateTerminationProtection, https response error ` +
		`StatusCode: 400, api error ValidationError: Stack [fixdemo-dev] does not exist`)

	err := wrapAPICallError("fixdemo-dev", awsErr)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.ErrorIs(t, err, awsErr)

	hints := cockroachErrors.GetAllHints(err)
	require.NotEmpty(t, hints, "error must carry at least one hint")
	joined := strings.Join(hints, " ")
	assert.Contains(t, joined, "--target")
	assert.Contains(t, joined, "apply")
}

// wrapAPICallError must leave every other AWS error shape as the existing
// plain sentinel wrap, without inventing a hint for an error it hasn't
// specifically recognized.
func TestWrapAPICallError_OtherError_PlainWrap(t *testing.T) {
	awsErr := errors.New("operation error CloudFormation: CreateChangeSet, access denied")

	err := wrapAPICallError("fixdemo-dev", awsErr)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
	assert.ErrorIs(t, err, awsErr)
	assert.Empty(t, cockroachErrors.GetAllHints(err), "an unrecognized AWS error must not get an invented hint")
}
