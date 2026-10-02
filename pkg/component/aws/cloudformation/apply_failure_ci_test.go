package cloudformation

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFailedApplyRetainsChangesetEvidence exercises real apply orchestration through
// its failure stages, then checks the aggregate handed to the CI plugin.
func TestFailedApplyRetainsChangesetEvidence(t *testing.T) {
	for _, stage := range []string{"compute", "decline", "policy", "execute", "poll", "rollback"} {
		t.Run(stage, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			spec := vpcSpec()
			octx := directApplyOctx(map[string]any{"auto-approve": true})
			described := completedChangeSet()
			described.ChangeSetId = awsString("reviewed-change-id")
			expectedError := errUtils.ErrAwsCloudFormationAPICallFailed
			if stage == "compute" {
				described.Status = cfntypes.ChangeSetStatusFailed
				described.StatusReason = awsString("Template validation failed")
				described.Changes = nil
				expectedError = errUtils.ErrAwsCloudFormationChangeSetFailed
			}
			calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), described)
			switch stage {
			case "decline":
				stubConfirmOperation(t, false, nil)
				octx.Flags = map[string]any{}
				expectedError = errUtils.ErrUserAborted
			case "policy":
				spec.StackPolicyBody = `{"Statement":[]}`
				calls = append(calls, client.EXPECT().SetStackPolicy(gomock.Any(), gomock.Any()).Return(nil, expectedError))
			case "execute", "poll", "rollback":
				calls = append(calls, client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil))
				if stage == "execute" {
					calls = append(calls, client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(nil, expectedError))
				} else {
					calls = append(calls, client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil))
					if stage == "poll" {
						calls = append(calls, client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, expectedError))
					} else {
						expectedError = errUtils.ErrAwsCloudFormationOperationFailed
						calls = append(calls,
							client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{StackEvents: []cfntypes.StackEvent{{EventId: awsString("new-failure"), ResourceStatus: cfntypes.ResourceStatusRollbackComplete}}}, nil),
							client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(stackWithStatus(cfntypes.StackStatusRollbackComplete), nil),
							client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil))
					}
				}
			}
			if stage != "poll" && stage != "rollback" {
				calls = append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))
			}
			gomock.InOrder(calls...)
			summary, err := runApply(octx, client, spec, map[string]any{"stack_name": "vpc"})
			require.ErrorIs(t, err, expectedError)
			assert.Equal(t, "reviewed-change-id", summary["changeset_id"])
			original := runCIHooks
			t.Cleanup(func() { runCIHooks = original })
			var captured *hooks.RunCIHooksOptions
			runCIHooks = func(options *hooks.RunCIHooksOptions) error { captured = options; return nil }
			runCIHook(ciHookParams{event: hooks.AfterAwsCloudFormationApply, summary: summary, commandErr: err})
			require.NotNil(t, captured)
			result := captured.Aggregate.(*schema.CloudFormationCIResult)
			assert.NotEmpty(t, result.ChangeSetName)
			assert.NotZero(t, result.ExitCode)
			assert.ErrorIs(t, captured.CommandError, expectedError)
			assert.Equal(t, stage != "compute", result.HasChanges)
			if stage == "compute" {
				assert.Zero(t, result.ResourceChanges)
			} else {
				assert.Equal(t, 1, result.ResourceChanges)
				assert.Contains(t, captured.Output, "Bucket")
			}
			if stage == "rollback" {
				assert.Equal(t, "ROLLBACK_COMPLETE", result.StackStatus)
			} else {
				assert.Empty(t, result.StackStatus)
			}
		})
	}
}
