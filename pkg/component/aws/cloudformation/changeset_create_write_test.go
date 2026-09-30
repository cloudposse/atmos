package cloudformation

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunChangesetCreate_ClosedStdout(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil)
	out, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, out.Close())
	old := os.Stdout
	os.Stdout = out
	t.Cleanup(func() { os.Stdout = old })
	_, err = runChangesetCreate(context.Background(), client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{})
	require.ErrorIs(t, err, os.ErrClosed)
}
