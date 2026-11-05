package cloudformation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Configured policies must protect the current update, and failures must stop execution.
func TestApplyStackPolicyExecutionOrdering(t *testing.T) {
	testStackPolicyExecutionOrdering(t, false, func(client CloudFormationClient, spec *stackSpec) error {
		_, err := runApply(&opContext{Ctx: context.Background(), AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{}, Flags: map[string]any{"auto-approve": true}}, client, spec, map[string]any{})
		return err
	})
}

// testStackPolicyExecutionOrdering checks policy and termination-protection ordering across create,
// update, and policy-failure paths for both apply entry points.
func testStackPolicyExecutionOrdering(t *testing.T, named bool, run func(CloudFormationClient, *stackSpec) error) {
	t.Helper()
	oldInterval := eventPollInterval
	eventPollInterval = time.Millisecond
	t.Cleanup(func() { eventPollInterval = oldInterval })
	for _, create := range []bool{false, true} {
		for _, failPolicy := range []bool{false, true} {
			name := "apply"
			if named {
				name = "named"
			}
			if create {
				name += "/create"
			} else {
				name += "/update"
			}
			if failPolicy {
				name += "/policy-error"
			}
			t.Run(name, func(t *testing.T) {
				client := NewMockCloudFormationClient(gomock.NewController(t))
				spec := &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}", TerminationProtection: true, StackPolicyBody: `{"Statement":[{"Effect":"Deny","Action":"Update:*","Principal":"*","Resource":"*"}]}`}
				initialStatus := cfntypes.StackStatusUpdateComplete
				progress, complete := cfntypes.StackStatusUpdateInProgress, cfntypes.StackStatusUpdateComplete
				if create {
					initialStatus = cfntypes.StackStatusReviewInProgress
					progress = cfntypes.StackStatusCreateInProgress
					complete = cfntypes.StackStatusCreateComplete
				}
				describeStack := func() *gomock.Call {
					return client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: initialStatus}}}, nil)
				}
				describeChangeset := func() *gomock.Call {
					return client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil)
				}
				var calls []any
				if named {
					calls = append(calls, describeChangeset(), describeStack())
				} else {
					calls = append(calls, describeStack(), client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil), describeChangeset())
				}
				policyErr := errors.New("policy permission denied")
				var expectedErr error
				if failPolicy {
					expectedErr = policyErr
				}
				policy := func() *gomock.Call {
					return client.EXPECT().SetStackPolicy(gomock.Any(), &cloudformation.SetStackPolicyInput{StackName: awsString("vpc"), StackPolicyBody: awsString(spec.StackPolicyBody)}).Return(&cloudformation.SetStackPolicyOutput{}, expectedErr)
				}
				if !create {
					calls = append(calls, policy())
					if failPolicy && !named {
						// apply abandons its own changeset when the policy cannot be set.
						calls = append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil))
					}
				}
				if create || !failPolicy {
					calls = append(calls,
						client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
						client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
						client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
						client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: progress}}}, nil),
						client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
						client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: complete}}}, nil),
						// Final read for the stack-level event after the terminal status.
						client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil))
					if create {
						calls = append(calls, policy())
					}
				}
				if !failPolicy {
					calls = append(calls, client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil))
				}
				if !failPolicy && !named {
					calls = append(calls, client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil))
				}

				gomock.InOrder(calls...)
				err := run(client, spec)
				if failPolicy {
					require.ErrorIs(t, err, policyErr)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestRunApplyNoOpReconcilesPolicy(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	spec := &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}", StackPolicyBody: "{}", TerminationProtection: true}
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}}}, nil),
		client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil),
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{ChangeSetId: awsString("cs-id"), Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("The submitted information didn't contain changes.")}, nil),
		client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, nil),
		client.EXPECT().SetStackPolicy(gomock.Any(), gomock.Any()).Return(&cloudformation.SetStackPolicyOutput{}, nil),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
	)
	summary, err := runApply(&opContext{Ctx: context.Background(), AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{}, Flags: map[string]any{"auto-approve": true}}, client, spec, map[string]any{})
	require.NoError(t, err)
	require.Equal(t, true, summary["no_op"])
	require.Equal(t, string(cfntypes.StackStatusUpdateComplete), summary["final_status"], "a no-op reports the stack's current status")
}

func TestApplyDryRunConfiguredStackPolicy(t *testing.T) {
	testDryRunConfiguredStackPolicy(t, OperationApply)
}

func testDryRunConfiguredStackPolicy(t *testing.T, operation Operation) {
	t.Helper()
	octx := &opContext{Ctx: context.Background(), Info: &schema.ConfigAndStacksInfo{DryRun: true}, Flags: map[string]any{}}
	_, err := runOperation(octx, operation, &stackSpec{StackName: "vpc", StackPolicyBody: "{}"})
	require.NoError(t, err)
}
