package cloudformation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNamedStackPolicyExecutionOrdering(t *testing.T) {
	testStackPolicyExecutionOrdering(t, true, func(client CloudFormationClient, spec *stackSpec) error {
		_, err := runChangesetExecute(context.Background(), client, spec, "reviewed", map[string]any{})
		return err
	})
}

func TestChangesetExecuteLoadsPolicyWithoutTemplate(t *testing.T) {
	dir := t.TempDir()
	policy := `{"Statement":[]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "policy.json"), []byte(policy), 0o600))
	stubProvisionAndResolveComponentPath(t, dir, nil)
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "vpc", "template": "absent-template.yaml", "stack_policy": map[string]any{"file": "policy.json"}}}
	spec, err := resolveSpecAndTemplate(context.Background(), &schema.AtmosConfiguration{}, info, OperationChangesetExecute)
	require.NoError(t, err)
	require.Equal(t, policy, spec.StackPolicyBody)
	require.Empty(t, spec.TemplateBody)
}

func TestNamedStackPolicyLookupErrorStopsExecution(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	lookupErr := errors.New("describe denied")
	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, lookupErr),
	)
	_, err := runChangesetExecute(context.Background(), client, &stackSpec{StackName: "vpc", StackPolicyBody: "{}"}, "reviewed", map[string]any{})
	require.ErrorIs(t, err, lookupErr)
}

func TestChangesetExecutePolicyLoadErrors(t *testing.T) {
	for _, provisioningFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing policy", true: "provisioning failure"}[provisioningFails], func(t *testing.T) {
			var provisionErr error
			if provisioningFails {
				provisionErr = errors.New("source unavailable")
			}
			stubProvisionAndResolveComponentPath(t, t.TempDir(), provisionErr)
			info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stack_name": "vpc", "template": "absent-template.yaml", "stack_policy": map[string]any{"file": "missing.json"}}}
			_, err := resolveSpecAndTemplate(context.Background(), &schema.AtmosConfiguration{}, info, OperationChangesetExecute)
			require.Error(t, err)
			if provisioningFails {
				require.ErrorIs(t, err, provisionErr)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestNamedDryRunConfiguredStackPolicy(t *testing.T) {
	testDryRunConfiguredStackPolicy(t, OperationChangesetExecute)
}

// Failed creation must never be followed by installing a stack policy.
func TestNamedCreationFailureDoesNotInstallPolicy(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	executeErr := errors.New("creation rejected")
	gomock.InOrder(
		client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusReviewInProgress}}}, nil),
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
		client.EXPECT().ExecuteChangeSet(gomock.Any(), gomock.Any()).Return(nil, executeErr),
	)
	_, err := runChangesetExecute(context.Background(), client, &stackSpec{StackName: "vpc", StackPolicyBody: "{}"}, "reviewed", map[string]any{})
	require.ErrorIs(t, err, executeErr)
}
