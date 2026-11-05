package cloudformation

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunWatch_AlreadyTerminalWithHistory(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{
			StackEvents: []cfntypes.StackEvent{{EventId: awsString("existing"), ResourceStatus: cfntypes.ResourceStatusUpdateComplete}},
		}, nil),
		client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{
			Stacks: []cfntypes.Stack{{StackStatus: cfntypes.StackStatusUpdateComplete}},
		}, nil),
		// Final read for the stack-level event after the terminal status.
		client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackEventsOutput{}, nil),
	)

	summary, err := runWatch(context.Background(), client, "root", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, string(cfntypes.StackStatusUpdateComplete), summary["final_status"])
}

func TestRunWatch_MissingStackIsNotSuccessfulDeletion(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	missing := &smithy.GenericAPIError{Code: "ValidationError", Message: "Stack with id root does not exist"}
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).Return(nil, missing)

	summary, err := runWatch(context.Background(), client, "root", map[string]any{})
	require.ErrorIs(t, err, missing)
	assert.NotContains(t, summary, "final_status")
}
