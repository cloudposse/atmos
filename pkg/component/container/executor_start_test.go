package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ctr "github.com/cloudposse/atmos/pkg/container"
)

// stoppedInfo is a discovered (by label) but exited container instance.
func stoppedInfo() []ctr.Info {
	return []ctr.Info{{ID: "cid", Status: "exited", Labels: ctr.InstanceLabels("dev", "container", "api")}}
}

func TestExecuteStart_StartsStopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(stoppedInfo(), nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(nil),
	)
	require.NoError(t, ExecuteStart(context.Background(), infoFor("api")))
}

func TestExecuteStart_AlreadyRunningNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	// Discovered as running => no Start call.
	rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(runningInfo(), nil)
	require.NoError(t, ExecuteStart(context.Background(), infoFor("api")))
}

func TestExecuteStart_StartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	gomock.InOrder(
		rt.EXPECT().List(gomock.Any(), ctr.DiscoveryFilter("dev", "container", "api")).Return(stoppedInfo(), nil),
		rt.EXPECT().Start(gomock.Any(), "cid").Return(assert.AnError),
	)
	require.Error(t, ExecuteStart(context.Background(), infoFor("api")))
}

// TestExecuteStart_NotFound covers a component with no existing container: start
// cannot create one (that is `up`), so discovery fails.
func TestExecuteStart_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	rt := NewMockRuntime(ctrl)
	withStubs(t, map[string]any{"image": "alpine"}, nil, rt)

	rt.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil) // nothing found
	require.Error(t, ExecuteStart(context.Background(), infoFor("api")))
}
