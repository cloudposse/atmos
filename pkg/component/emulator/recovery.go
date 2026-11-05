package emulator

import (
	"context"

	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/schema"
)

func (r *resolved) manager() emulatorManager {
	manager := newManager(r.runtimePref, r.autoStart)
	setManagerRetry(manager, r.retry)
	return manager
}

func setManagerRetry(manager emulatorManager, runner *recovery.Runner) {
	if runner != nil {
		if setter, ok := manager.(interface {
			SetOperationRetry(container.OperationRetry)
		}); ok {
			setter.SetOperationRetry(func(ctx context.Context, stage string, fn func() error) error {
				return recovery.Do(ctx, runner, stage, fn)
			})
		}
	}
}

func executionCommand(command string, fallback []string) string {
	if command == "" && len(fallback) > 0 {
		return fallback[0]
	}
	return command
}

func executionRecovery(info *schema.ConfigAndStacksInfo, command string) (*recovery.Runner, error) {
	if command == "" {
		return nil, nil
	}
	return recovery.New(info, "emulator", command)
}
