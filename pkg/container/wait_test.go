package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

var waitInstance = Instance{Stack: "dev", ComponentType: "emulator", Component: "aws"}

func healthInfo(health string) Info {
	return Info{
		ID:     "abc123",
		Status: "running",
		Health: health,
		Labels: map[string]string{
			LabelInstance: InstanceAddress(waitInstance.Stack, waitInstance.ComponentType, waitInstance.Component),
		},
	}
}

func TestWaitHealthy(t *testing.T) {
	t.Run("nil runtime", func(t *testing.T) {
		err := WaitHealthy(context.Background(), nil, waitInstance, time.Second)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})

	t.Run("returns nil once healthy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		runtime := NewMockRuntime(ctrl)
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]Info{healthInfo("healthy")}, nil).AnyTimes()

		err := WaitHealthy(context.Background(), runtime, waitInstance, time.Second)
		assert.NoError(t, err)
	})

	t.Run("fails fast when unhealthy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		runtime := NewMockRuntime(ctrl)
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]Info{healthInfo("unhealthy")}, nil).AnyTimes()

		err := WaitHealthy(context.Background(), runtime, waitInstance, 5*time.Second)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrContainerNotHealthy)
	})

	t.Run("times out while still starting", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		runtime := NewMockRuntime(ctrl)
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]Info{healthInfo("starting")}, nil).AnyTimes()

		err := WaitHealthy(context.Background(), runtime, waitInstance, time.Millisecond)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrContainerNotHealthy)
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		runtime := NewMockRuntime(ctrl)
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).
			Return([]Info{healthInfo("starting")}, nil).AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := WaitHealthy(ctx, runtime, waitInstance, time.Minute)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
