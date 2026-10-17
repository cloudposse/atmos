package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDescribeStackOutputs(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)

	vpcID := "VpcId"
	vpcVal := "vpc-0123456789"
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
		Stacks: []cfntypes.Stack{{
			Outputs: []cfntypes.Output{
				{OutputKey: &vpcID, OutputValue: &vpcVal},
			},
		}},
	}, nil)

	outputs, err := describeStackOutputs(context.Background(), client, "vpc")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"VpcId": "vpc-0123456789"}, outputs)
}

func TestDescribeStackOutputs_NoStack(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)

	outputs, err := describeStackOutputs(context.Background(), client, "vpc")
	require.NoError(t, err)
	assert.Empty(t, outputs)
}
