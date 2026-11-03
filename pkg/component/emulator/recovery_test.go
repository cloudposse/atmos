package emulator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
)

type retryManagerProbe struct {
	emulatorManager
	retry container.OperationRetry
}

func (m *retryManagerProbe) SetOperationRetry(retry container.OperationRetry) { m.retry = retry }

func TestEmulatorManagerReceivesOperationRetry(t *testing.T) {
	recoverytest.Install(t, "emulator.up.error", "input.stage == 'Start' && input.error.message == 'transient'")
	runner, err := recovery.New(nil, "emulator", "up")
	require.NoError(t, err)
	probe := &retryManagerProbe{}
	original := newManager
	t.Cleanup(func() { newManager = original })
	newManager = func(preference string, autoStart bool) emulatorManager {
		require.Equal(t, "podman", preference)
		require.True(t, autoStart)
		return probe
	}
	resolved := &resolved{runtimePref: "podman", autoStart: true, retry: runner}
	require.Same(t, probe, resolved.manager())
	require.NotNil(t, probe.retry)
	calls := 0
	require.NoError(t, probe.retry(context.Background(), "Start", func() error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	}))
	require.Equal(t, 2, calls)
}
