package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRecoveryPollDoesNotRepeatExecutedChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.error.code == 'Throttling'")
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.ExecuteChangeSetOutput{}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, &smithy.GenericAPIError{Code: "Throttling", Message: "slow down"}),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil),
	)
	original := operationHandlers[OperationApply]
	t.Cleanup(func() { operationHandlers[OperationApply] = original })
	operationHandlers[OperationApply] = func(octx *opContext, client CloudFormationClient, _ *stackSpec, summary map[string]any) (map[string]any, error) {
		if _, err := client.ExecuteChangeSet(octx.Ctx, &cloudformation.ExecuteChangeSetInput{}); err != nil {
			return nil, err
		}
		_, err := client.DescribeStacks(octx.Ctx, &cloudformation.DescribeStacksInput{})
		return summary, err
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	_, err := runRemoteOperation(&opContext{Ctx: context.Background(), Info: info}, OperationApply, client, &stackSpec{}, map[string]any{})
	require.NoError(t, err)
}

// Every API adapter must preserve its arguments, options and typed response,
// and select the retry policy using the actual SDK method as its stage.
func TestRecoveryClientCreateChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'CreateChangeSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.CreateChangeSet, client.EXPECT().CreateChangeSet)
}

func TestRecoveryClientDescribeChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeChangeSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeChangeSet, client.EXPECT().DescribeChangeSet)
}

func TestRecoveryClientDeleteChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DeleteChangeSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DeleteChangeSet, client.EXPECT().DeleteChangeSet)
}

func TestRecoveryClientExecuteChangeSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ExecuteChangeSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ExecuteChangeSet, client.EXPECT().ExecuteChangeSet)
}

func TestRecoveryClientListChangeSets(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ListChangeSets' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ListChangeSets, client.EXPECT().ListChangeSets)
}

func TestRecoveryClientDescribeStacks(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStacks' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStacks, client.EXPECT().DescribeStacks)
}

func TestRecoveryClientDescribeStackEvents(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStackEvents' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStackEvents, client.EXPECT().DescribeStackEvents)
}

func TestRecoveryClientDeleteStack(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DeleteStack' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DeleteStack, client.EXPECT().DeleteStack)
}

func TestRecoveryClientValidateTemplate(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ValidateTemplate' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ValidateTemplate, client.EXPECT().ValidateTemplate)
}

func TestRecoveryClientUpdateTerminationProtection(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'UpdateTerminationProtection' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.UpdateTerminationProtection, client.EXPECT().UpdateTerminationProtection)
}

func TestRecoveryClientSetStackPolicy(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'SetStackPolicy' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.SetStackPolicy, client.EXPECT().SetStackPolicy)
}

func TestRecoveryClientGetTemplate(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'GetTemplate' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.GetTemplate, client.EXPECT().GetTemplate)
}

func TestRecoveryClientGetStackPolicy(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'GetStackPolicy' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.GetStackPolicy, client.EXPECT().GetStackPolicy)
}

func TestRecoveryClientListStacks(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ListStacks' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ListStacks, client.EXPECT().ListStacks)
}

func TestRecoveryClientDetectStackDrift(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DetectStackDrift' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DetectStackDrift, client.EXPECT().DetectStackDrift)
}

func TestRecoveryClientDescribeStackDriftDetectionStatus(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStackDriftDetectionStatus' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStackDriftDetectionStatus, client.EXPECT().DescribeStackDriftDetectionStatus)
}

func TestRecoveryClientDescribeStackResourceDrifts(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStackResourceDrifts' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStackResourceDrifts, client.EXPECT().DescribeStackResourceDrifts)
}

func TestRecoveryClientDescribeStackSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStackSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStackSet, client.EXPECT().DescribeStackSet)
}

func TestRecoveryClientCreateStackSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'CreateStackSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.CreateStackSet, client.EXPECT().CreateStackSet)
}

func TestRecoveryClientUpdateStackSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'UpdateStackSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.UpdateStackSet, client.EXPECT().UpdateStackSet)
}

func TestRecoveryClientDeleteStackSet(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DeleteStackSet' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DeleteStackSet, client.EXPECT().DeleteStackSet)
}

func TestRecoveryClientCreateStackInstances(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'CreateStackInstances' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.CreateStackInstances, client.EXPECT().CreateStackInstances)
}

func TestRecoveryClientDeleteStackInstances(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DeleteStackInstances' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DeleteStackInstances, client.EXPECT().DeleteStackInstances)
}

func TestRecoveryClientListStackInstances(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ListStackInstances' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ListStackInstances, client.EXPECT().ListStackInstances)
}

func TestRecoveryClientDescribeStackSetOperation(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'DescribeStackSetOperation' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.DescribeStackSetOperation, client.EXPECT().DescribeStackSetOperation)
}

func TestRecoveryClientListStackResources(t *testing.T) {
	recoverytest.Install(t, "aws/cloudformation.apply.error", "input.stage == 'ListStackResources' && input.error.code == 'Throttling'")
	runner, err := recovery.New(nil, "aws/cloudformation", "apply")
	require.NoError(t, err)
	client := NewMockCloudFormationClient(gomock.NewController(t))
	wrapped := &recoveryClient{CloudFormationClient: client, runner: runner}
	verifyRecoveryMethod(t, wrapped.ListStackResources, client.EXPECT().ListStackResources)
}

func verifyRecoveryMethod[I, O any](t *testing.T,
	invoke func(context.Context, *I, ...func(*cloudformation.Options)) (*O, error),
	expect func(any, any, ...any) *gomock.Call,
) {
	t.Helper()
	ctx := t.Context()
	input, output := new(I), new(O)
	attempts := 0
	failure := &smithy.GenericAPIError{Code: "Throttling", Message: "slow down"}
	expect(ctx, input, gomock.Any()).DoAndReturn(func(actualCtx context.Context, actual *I, opts ...func(*cloudformation.Options)) (*O, error) {
		require.Same(t, input, actual)
		require.Equal(t, ctx, actualCtx)
		require.Len(t, opts, 1)
		var options cloudformation.Options
		opts[0](&options)
		require.Equal(t, "test-region", options.Region)
		attempts++
		if attempts == 1 {
			return nil, failure
		}
		return output, nil
	}).Times(2)
	result, err := invoke(ctx, input, func(opts *cloudformation.Options) { opts.Region = "test-region" })
	require.NoError(t, err)
	require.Same(t, output, result)
	require.Equal(t, 2, attempts)
	// Cancellation must not reach the SDK, even with retry policies installed.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	result, err = invoke(canceled, input)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}
