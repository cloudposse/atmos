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

// listChangeSets must paginate through every NextToken until CloudFormation
// stops returning one, accumulating every page's Summaries.
func TestListChangeSets_Paginates(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	token := "page-2"
	gomock.InOrder(
		client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.ListChangeSetsInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error) {
				assert.Nil(t, input.NextToken, "the first page request must not carry a token")
				return &cloudformation.ListChangeSetsOutput{
					Summaries: []cfntypes.ChangeSetSummary{{ChangeSetName: awsString("cs-1")}},
					NextToken: &token,
				}, nil
			},
		),
		client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, input *cloudformation.ListChangeSetsInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error) {
				assert.Equal(t, &token, input.NextToken, "the second page request must carry the first page's token")
				return &cloudformation.ListChangeSetsOutput{
					Summaries: []cfntypes.ChangeSetSummary{{ChangeSetName: awsString("cs-2")}},
				}, nil
			},
		),
	)

	summaries, err := listChangeSets(context.Background(), client, "vpc")
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, "cs-1", stringValue(summaries[0].ChangeSetName))
	assert.Equal(t, "cs-2", stringValue(summaries[1].ChangeSetName))
}

// listChangeSets must wrap a ListChangeSets API error.
func TestListChangeSets_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := listChangeSets(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// deleteChangeSet must succeed on the happy path and wrap an API error.
func TestDeleteChangeSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil)

	err := deleteChangeSet(context.Background(), client, "vpc", "cs-1")
	require.NoError(t, err)
}

func TestDeleteChangeSet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	err := deleteChangeSet(context.Background(), client, "vpc", "cs-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

// describeNamedChangeSet must return a populated changeSetResult on success.
func TestDescribeNamedChangeSet_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		ChangeSetId: awsString("cs-id-1"),
		StackId:     awsString("stack-id-1"),
		Status:      cfntypes.ChangeSetStatusCreateComplete,
		Changes:     []cfntypes.Change{{Type: cfntypes.ChangeTypeResource}},
	}, nil)

	result, err := describeNamedChangeSet(context.Background(), client, "vpc", "cs-1")
	require.NoError(t, err)
	assert.Equal(t, "cs-id-1", result.ChangeSetID)
	assert.Equal(t, "cs-1", result.ChangeSetName)
	assert.Equal(t, "stack-id-1", result.StackID)
	assert.Equal(t, cfntypes.ChangeSetStatusCreateComplete, result.Status)
	assert.Len(t, result.Changes, 1)
}

// describeNamedChangeSet must translate a "not found" DescribeChangeSet error
// into ErrAwsCloudFormationChangeSetNotFound, distinct from a generic failure.
func TestDescribeNamedChangeSet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("ChangeSet [cs-1] does not exist"))

	_, err := describeNamedChangeSet(context.Background(), client, "vpc", "cs-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetNotFound)
	assert.Contains(t, err.Error(), "cs-1")
	assert.Contains(t, err.Error(), "vpc")
}

// describeNamedChangeSet must wrap any other API error as a generic failure,
// not the not-found sentinel.
func TestDescribeNamedChangeSet_OtherError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	_, err := describeNamedChangeSet(context.Background(), client, "vpc", "cs-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
	assert.NotErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetNotFound)
}

// runChangesetCreate must render the diff summary and populate the summary
// map from the created changeset, leaving it in place (no execute call).
func TestRunChangesetCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
	}, nil)

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	out := captureStdout(t, func() {
		summary, err := runChangesetCreate(context.Background(), client, spec, map[string]any{})
		require.NoError(t, err)
		assert.False(t, summary["no_op"].(bool))
		assert.NotEmpty(t, summary["changeset_name"])
	})
	assert.Contains(t, out, "changeset:")
}

// runChangesetCreate must propagate a createChangeSet failure.
func TestRunChangesetCreate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, errors.New("access denied"))

	spec := &stackSpec{StackName: "vpc", TemplateBody: "AWSTemplateFormatVersion: '2010-09-09'"}
	_, err := runChangesetCreate(context.Background(), client, spec, map[string]any{})
	require.Error(t, err)
}

// runChangesetExecute's happy path: describe the named changeset, execute it,
// stream events to a terminal success status.
func TestRunChangesetExecute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			ChangeSetId: awsString("cs-id-1"),
			Status:      cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
		}, nil),
	)

	spec := &stackSpec{StackName: "vpc"}
	summary, err := runChangesetExecute(context.Background(), client, spec, "cs-1", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "cs-id-1", summary["changeset_id"])
	assert.Equal(t, string(cfntypes.StackStatusUpdateComplete), summary["final_status"])
}

// runChangesetExecute must propagate a describeNamedChangeSet failure without
// ever calling ExecuteChangeSet.
func TestRunChangesetExecute_DescribeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetExecute(context.Background(), client, spec, "cs-1", map[string]any{})
	require.Error(t, err)
}

// runChangesetExecute must propagate an executeChangeSet failure without
// streaming events.
func TestRunChangesetExecute_ExecuteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
		Status: cfntypes.ChangeSetStatusCreateComplete,
	}, nil)
	client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetExecute(context.Background(), client, spec, "cs-1", map[string]any{})
	require.Error(t, err)
}

// runChangesetExecute must propagate a streamStackEvents failure (e.g. the
// DescribeStackEvents API call itself failing) without misreporting it as a
// terminal stack status.
func TestRunChangesetExecute_StreamEventsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status: cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled")),
	)

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetExecute(context.Background(), client, spec, "cs-1", map[string]any{})
	require.Error(t, err)
}

// runChangesetExecute must report a failed terminal stack status as an error.
func TestRunChangesetExecute_FailedStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{
			Status: cfntypes.ChangeSetStatusCreateComplete,
		}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateRollbackComplete}},
		}, nil),
	)

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetExecute(context.Background(), client, spec, "cs-1", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationOperationFailed)
}

// runChangesetList must render a "no changesets" line and leave summary's
// changesets empty when the stack has none.
func TestRunChangesetList_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).Return(&cloudformation.ListChangeSetsOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	out := captureStdout(t, func() {
		summary, err := runChangesetList(context.Background(), client, spec, map[string]any{})
		require.NoError(t, err)
		assert.Empty(t, summary["changesets"])
	})
	assert.Contains(t, out, "vpc: no changesets")
}

// runChangesetList must render one line per changeset and populate the
// summary with every entry.
func TestRunChangesetList_Populated(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).Return(&cloudformation.ListChangeSetsOutput{
		Summaries: []cfntypes.ChangeSetSummary{
			{ChangeSetName: awsString("cs-1"), Status: cfntypes.ChangeSetStatusCreateComplete, Description: awsString("first")},
			{ChangeSetName: awsString("cs-2"), Status: cfntypes.ChangeSetStatusFailed, Description: awsString("second")},
		},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	out := captureStdout(t, func() {
		summary, err := runChangesetList(context.Background(), client, spec, map[string]any{})
		require.NoError(t, err)
		assert.Len(t, summary["changesets"].([]cfntypes.ChangeSetSummary), 2)
	})
	assert.Contains(t, out, "cs-1")
	assert.Contains(t, out, "cs-2")
	assert.Contains(t, out, "first")
	assert.Contains(t, out, "second")
}

// runChangesetList must propagate a listChangeSets failure.
func TestRunChangesetList_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListChangeSets(gomock.Any(), gomock.Any()).Return(nil, errors.New("throttled"))

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetList(context.Background(), client, spec, map[string]any{})
	require.Error(t, err)
}

// runChangesetDelete must delete the named changeset and report success.
func TestRunChangesetDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	out := captureStdout(t, func() {
		summary, err := runChangesetDelete(context.Background(), client, spec, "cs-1", map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "cs-1", summary["changeset_name"])
	})
	assert.Contains(t, out, `changeset "cs-1" deleted`)
}

// runChangesetDelete must propagate a deleteChangeSet failure.
func TestRunChangesetDelete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	spec := &stackSpec{StackName: "vpc"}
	_, err := runChangesetDelete(context.Background(), client, spec, "cs-1", map[string]any{})
	require.Error(t, err)
}
