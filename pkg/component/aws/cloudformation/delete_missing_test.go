package cloudformation

import (
	"testing"

	sdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunDelete_MissingStackIsIdempotent(t *testing.T) {
	for _, flags := range []map[string]any{nil, {"retain-resources": []string{"Bucket"}}, {"disable-termination-protection": true}} {
		t.Run("delete", func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}
			gomock.InOrder(
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, missing),
				client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(nil, missing),
				client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, missing),
			)
			summary, err := runDelete(t.Context(), client, flags, &stackSpec{StackName: "vpc"}, map[string]any{})
			require.NoError(t, err)
			require.Equal(t, string(cfntypes.StackStatusDeleteComplete), summary["final_status"])
		})
	}
}

func TestDeleteStack_PreservesOtherAWSErrors(t *testing.T) {
	for _, apiErr := range []*smithy.GenericAPIError{
		{Code: "AccessDenied", Message: "Stack with id vpc does not exist or access is denied"},
		{Code: "ValidationError", Message: "Invalid stack name"},
	} {
		t.Run(apiErr.Code, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return((*sdk.DescribeStacksOutput)(nil), apiErr)
			require.ErrorIs(t, deleteStack(t.Context(), client, &stackSpec{StackName: "vpc"}, deleteOptions{}), apiErr)
		})
	}
}

func TestDeleteStack_DisappearsAfterProtectionDisabled(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id vpc does not exist"}
	gomock.InOrder(
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&sdk.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{EnableTerminationProtection: awsBool(true)}},
		}, nil),
		client.EXPECT().UpdateTerminationProtection(gomock.Any(), gomock.Any()).Return(&sdk.UpdateTerminationProtectionOutput{}, nil),
		client.EXPECT().DeleteStack(gomock.Any(), gomock.Any()).Return(nil, missing),
	)
	require.NoError(t, deleteStack(t.Context(), client, &stackSpec{StackName: "vpc"}, deleteOptions{DisableTerminationProtection: true}))
}
