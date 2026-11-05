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

// stackNameFromID must return the stack name from an ARN, and anything else unchanged.
func TestStackNameFromID(t *testing.T) {
	tests := []struct{ name, id, want string }{
		{name: "arn", id: "arn:aws:cloudformation:us-east-2:123456789012:stack/app-network/5f0c-guid", want: "app-network"},
		{name: "bare name", id: "app", want: "app"},
		{name: "arn without stack resource", id: "arn:aws:s3:::bucket", want: "arn:aws:s3:::bucket"},
		{name: "arn with empty name", id: "arn:aws:cloudformation:us-east-2:1:stack//guid", want: "arn:aws:cloudformation:us-east-2:1:stack//guid"},
		{name: "empty", id: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stackNameFromID(tt.id))
		})
	}
}

// The tree must show the logical ID and stack name of a nested stack, never its ARN, while the
// node keeps the ARN as its API identifier so descendants can still be walked.
func TestRunTree_ShowsLogicalIDAndNameNotARN(t *testing.T) {
	const childARN = "arn:aws:cloudformation:us-east-2:123456789012:stack/root-Network-ABC/guid-1"
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().ListStackResources(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *cloudformation.ListStackResourcesInput, _ ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
			if *in.StackName == "root" {
				return &cloudformation.ListStackResourcesOutput{StackResourceSummaries: []cfntypes.StackResourceSummary{
					{ResourceType: awsString(nestedStackResourceType), LogicalResourceId: awsString("Network"), PhysicalResourceId: awsString(childARN)},
				}}, nil
			}
			assert.Equal(t, childARN, *in.StackName, "descendants are walked by ARN")
			return &cloudformation.ListStackResourcesOutput{}, nil
		},
	).Times(2)

	var tree *stackNode
	out := captureStdout(t, func() {
		summary, err := runTree(context.Background(), client, "root", map[string]any{})
		require.NoError(t, err)
		tree = summary["tree"].(*stackNode)
	})

	assert.Equal(t, "root\n└─ Network (root-Network-ABC)\n", out)
	assert.NotContains(t, out, "arn:aws")
	require.Len(t, tree.Children, 1)
	assert.Equal(t, childARN, tree.Children[0].StackName)
	assert.Equal(t, "Network", tree.Children[0].LogicalID)
	assert.Equal(t, []string{"root", childARN}, flattenStackNames(tree))
}

// A node whose logical ID equals its stack name must not repeat itself, and a root has no logical ID.
func TestStackNodeLabel(t *testing.T) {
	assert.Equal(t, "root", (&stackNode{StackName: "root"}).label())
	assert.Equal(t, "same", (&stackNode{StackName: "same", LogicalID: "same"}).label())
	assert.Equal(t, "Net (stack)", (&stackNode{StackName: "stack", LogicalID: "Net"}).label())
}
