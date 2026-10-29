package emulator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
)

func TestManagerRetryDoesNotRepeatSuccessfulStart(t *testing.T) {
	recoverytest.Install(t, "emulator.up.error", "input.error.message.contains('transient')")
	runner, err := recovery.New(nil, "emulator", "up")
	require.NoError(t, err)
	runtime := NewMockRuntime(gomock.NewController(t))
	gomock.InOrder(
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, nil),
		runtime.EXPECT().Create(gomock.Any(), gomock.Any()).Return("new-id", nil),
		runtime.EXPECT().Start(gomock.Any(), "new-id").Return(nil),
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return(nil, errors.New("transient")),
		runtime.EXPECT().List(gomock.Any(), gomock.Any()).Return([]container.Info{runningEmulatorInfo(40001)}, nil),
		runtime.EXPECT().Inspect(gomock.Any(), "container-abc").Return(&container.Info{}, nil),
	)
	manager := newManagerWithRuntime(runtime)
	manager.SetOperationRetry(func(ctx context.Context, stage string, invoke func() error) error {
		return recovery.Do(ctx, runner, stage, invoke)
	})
	endpoint, err := manager.Up(context.Background(), &Spec{Driver: testDriverName}, "dev", "aws", nil)
	require.NoError(t, err)
	require.Equal(t, 40001, endpoint.Ports[4566])
}
