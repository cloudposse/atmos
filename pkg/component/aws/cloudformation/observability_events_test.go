package cloudformation

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestListAllStackEvents_Pages(t *testing.T) {
	t.Parallel()

	for _, terminal := range []*string{nil, awsString("")} {
		name := "nil token"
		if terminal != nil {
			name = "empty token"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := NewMockCloudFormationClient(gomock.NewController(t))
			overlap := cfntypes.StackEvent{EventId: awsString("overlap"), LogicalResourceId: awsString("Overlap")}
			gomock.InOrder(
				client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStackEventsOutput{
					StackEvents: []cfntypes.StackEvent{{EventId: awsString("newest"), LogicalResourceId: awsString("Newest")}, overlap},
					NextToken:   awsString("empty-page"),
				}, nil),
				client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root"), NextToken: awsString("empty-page")}).Return(&cloudformation.DescribeStackEventsOutput{
					NextToken: awsString("oldest-page"),
				}, nil),
				client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root"), NextToken: awsString("oldest-page")}).Return(&cloudformation.DescribeStackEventsOutput{
					StackEvents: []cfntypes.StackEvent{
						overlap,
						{EventId: awsString("oldest"), LogicalResourceId: awsString("Oldest")},
						{LogicalResourceId: awsString("NoID-A")},
						{LogicalResourceId: awsString("NoID-B")},
					},
					NextToken: terminal,
				}, nil),
			)

			events, err := listAllStackEvents(context.Background(), client, "root")
			require.NoError(t, err)
			var names []string
			for _, event := range events {
				names = append(names, stringValue(event.LogicalResourceId))
			}
			assert.Equal(t, []string{"NoID-B", "NoID-A", "Oldest", "Overlap", "Newest"}, names)
		})
	}
}

func TestListAllStackEvents_Error(t *testing.T) {
	t.Parallel()

	for _, afterPage := range []bool{false, true} {
		t.Run(fmt.Sprintf("afterPage=%t", afterPage), func(t *testing.T) {
			t.Parallel()
			client := NewMockCloudFormationClient(gomock.NewController(t))
			apiErr := &smithy.GenericAPIError{Code: "AccessDenied", Message: "access denied"}
			wrapped := fmt.Errorf("DescribeStackEvents: %w", apiErr)
			var token *string
			if afterPage {
				token = awsString("next")
				client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root")}).Return(&cloudformation.DescribeStackEventsOutput{
					StackEvents: []cfntypes.StackEvent{{EventId: awsString("event")}},
					NextToken:   token,
				}, nil)
			}
			client.EXPECT().DescribeStackEvents(gomock.Any(), &cloudformation.DescribeStackEventsInput{StackName: awsString("root"), NextToken: token}).Return(nil, wrapped)

			events, err := listAllStackEvents(context.Background(), client, "root")
			assert.Nil(t, events, "an incomplete history must not be returned as successful output")
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationAPICallFailed)
			require.ErrorIs(t, err, wrapped)
			require.ErrorIs(t, err, apiErr)
			var got smithy.APIError
			require.ErrorAs(t, err, &got)
			assert.Same(t, apiErr, got)
		})
	}
}
