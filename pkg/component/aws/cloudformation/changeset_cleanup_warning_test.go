package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestDeployDirect_CleanupWarnings preserves apply's original result when best-effort cleanup fails.
func TestDeployDirect_CleanupWarnings(t *testing.T) {
	for _, noOp := range []bool{false, true} {
		name := "declined"
		if noOp {
			name = "no-op"
		}
		t.Run(name, func(t *testing.T) {
			stubConfirmOperation(t, false, nil)
			client := NewMockCloudFormationClient(gomock.NewController(t))
			described := completedChangeSet()
			if noOp {
				described = &cloudformation.DescribeChangeSetOutput{Status: cfntypes.ChangeSetStatusFailed, StatusReason: awsString("No updates are to be performed")}
			}
			calls := expectChangeSetCreated(client, stackWithStatus(cfntypes.StackStatusUpdateComplete), described)
			cleanupErr := errors.New("cleanup permission denied")
			gomock.InOrder(append(calls, client.EXPECT().DeleteChangeSet(gomock.Any(), gomock.Any()).Return(nil, cleanupErr))...)
			var err error
			output := captureStderr(t, func() {
				_, err = deployDirect(&opContext{Ctx: context.Background()}, client, vpcSpec())
			})
			if noOp {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errUtils.ErrUserAborted)
			}
			assert.Contains(t, ansi.Strip(output), cleanupErr.Error())
		})
	}
}
