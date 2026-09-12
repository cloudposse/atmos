package cloudformation

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRunDiff_ClosedStdout(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cleanupErr error
	}{
		{name: "cleanup succeeds"},
		{name: "cleanup fails", cleanupErr: errors.New("cleanup unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewMockCloudFormationClient(gomock.NewController(t))
			client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStacksOutput{}, nil)
			client.EXPECT().CreateChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.CreateChangeSetOutput{}, nil)
			client.EXPECT().DescribeChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusCreateComplete}, nil)
			client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(&cloudformation.DeleteChangeSetOutput{}, tc.cleanupErr)
			out, err := os.CreateTemp(t.TempDir(), "closed")
			require.NoError(t, err)
			require.NoError(t, out.Close())
			old := os.Stdout
			os.Stdout = out
			t.Cleanup(func() { os.Stdout = old })
			_, err = runDiff(&opContext{Ctx: context.Background()}, client, &stackSpec{StackName: "vpc", TemplateBody: "Resources: {}"}, map[string]any{})
			require.ErrorIs(t, err, os.ErrClosed)
		})
	}
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
