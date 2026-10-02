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

func TestRunDiff_ClosedStdout(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockCloudFormationClient(ctrl)
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
	client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
	client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil)
	out, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, out.Close())
	old := os.Stdout
	os.Stdout = out
	t.Cleanup(func() { os.Stdout = old })
	_, err = runDiff(&opContext{Ctx: context.Background()}, client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{})
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestRenderDiffSummary_NoOpClosedStdout(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, out.Close())
	old := os.Stdout
	os.Stdout = out
	t.Cleanup(func() { os.Stdout = old })
	require.ErrorIs(t, renderDiffSummary("vpc", &changeSetResult{NoOp: true}), os.ErrClosed)
}
