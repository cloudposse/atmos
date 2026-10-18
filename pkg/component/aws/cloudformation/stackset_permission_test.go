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

func TestRunStackSetCreate_ServiceManagedInstancesRejectBeforeMutation(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Times(0)
	client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Times(0)
	cfg := &stackSetConfig{PermissionModel: "SERVICE_MANAGED", Accounts: []string{"111111111111"}, Regions: []string{"us-east-1"}}
	summary, err := runStackSetCreate(context.Background(), client, &stackSpec{StackName: "mine"}, cfg, map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.Contains(t, err.Error(), "SERVICE_MANAGED")
	assert.Empty(t, summary)
}

func TestRunStackSetCreate_ServiceManagedWithoutInstances(t *testing.T) {
	tests := []struct {
		name              string
		accounts, regions []string
	}{
		{name: "neither"},
		{name: "accounts only", accounts: []string{"111111111111"}},
		{name: "regions only", regions: []string{"us-east-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *cloudformation.CreateStackSetInput, _ ...func(*cloudformation.Options)) (*cloudformation.CreateStackSetOutput, error) {
				assert.Equal(t, cfntypes.PermissionModelsServiceManaged, input.PermissionModel)
				return &cloudformation.CreateStackSetOutput{}, nil
			})
			client.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Times(0)
			cfg := &stackSetConfig{PermissionModel: "SERVICE_MANAGED", Accounts: tt.accounts, Regions: tt.regions}
			summary, err := runStackSetCreate(context.Background(), client, &stackSpec{StackName: "mine"}, cfg, map[string]any{})
			require.NoError(t, err)
			assert.Equal(t, "mine", summary["stackset_name"])
		})
	}
}

func TestRunStackSetDelete_ServiceManagedInstancesRejectBeforeMutation(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().DescribeStackSet(gomock.Any(), &cloudformation.DescribeStackSetInput{StackSetName: awsString("mine")}).Return(&cloudformation.DescribeStackSetOutput{StackSet: &cfntypes.StackSet{PermissionModel: cfntypes.PermissionModelsServiceManaged}}, nil),
		client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{Summaries: []cfntypes.StackInstanceSummary{{Account: awsString("111111111111"), Region: awsString("us-east-1")}}}, nil),
	)
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Times(0)
	client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Times(0)
	summary, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	assert.Contains(t, err.Error(), "SERVICE_MANAGED")
	assert.Empty(t, summary)
}

func TestRunStackSetDelete_ServiceManagedEmpty(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().DescribeStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOutput{StackSet: &cfntypes.StackSet{PermissionModel: cfntypes.PermissionModelsServiceManaged}}, nil),
		client.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{}, nil),
		client.EXPECT().DeleteStackSet(gomock.Any(), &cloudformation.DeleteStackSetInput{StackSetName: awsString("mine")}).Return(&cloudformation.DeleteStackSetOutput{}, nil),
	)
	client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Times(0)
	summary, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "mine", summary["stackset_name"])
}

func TestRunStackSetDelete_DescribeErrorStopsMutation(t *testing.T) {
	apiErr := errors.New("access denied")
	tests := []struct {
		name     string
		response *cloudformation.DescribeStackSetOutput
		err      error
	}{
		{name: "API error", err: apiErr},
		{name: "missing stackset", response: &cloudformation.DescribeStackSetOutput{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStackSet(gomock.Any(), gomock.Any()).Return(tt.response, tt.err)
			client.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Times(0)
			client.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Times(0)
			summary, err := runStackSetDelete(context.Background(), client, "mine", map[string]any{})
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackSetFailed)
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
			}
			assert.Empty(t, summary)
		})
	}
}
