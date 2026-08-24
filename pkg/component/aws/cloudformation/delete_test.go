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

func TestDeleteStack_BlocksOnTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	// No API calls expected — the guard must short-circuit before calling DeleteStack.

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

func TestDeleteStack_DisableTerminationProtectionFlag(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateTerminationProtectionOutput{}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc", TerminationProtection: true}
	err := deleteStack(context.Background(), client, spec, deleteOptions{DisableTerminationProtection: true})
	require.NoError(t, err)
}

func TestDeleteStack_NoTerminationProtection(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteStackOutput{}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{})
	require.NoError(t, err)
}

func TestDeleteStack_RetainResourcesRequiresDeleteFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
	}, nil)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{RetainResources: []string{"MyBucket"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

func TestDeleteStack_RetainResourcesAllowedWhenDeleteFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusDeleteFailed}},
	}, nil)
	client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *cloudformation.DeleteStackInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error) {
			assert.Equal(t, []string{"MyBucket"}, input.RetainResources)
			return &cloudformation.DeleteStackOutput{}, nil
		},
	)

	spec := &stackSpec{StackName: "vpc"}
	err := deleteStack(context.Background(), client, spec, deleteOptions{RetainResources: []string{"MyBucket"}})
	require.NoError(t, err)
}

func TestDisableTerminationProtection_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	err := disableTerminationProtection(context.Background(), client, "vpc")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAwsCloudFormationChangeSetFailed)
}

func TestIsDeleteFailedStack(t *testing.T) {
	assert.True(t, isDeleteFailedStack(cfntypes.StackStatusDeleteFailed))
	assert.False(t, isDeleteFailedStack(cfntypes.StackStatusUpdateComplete))
}
