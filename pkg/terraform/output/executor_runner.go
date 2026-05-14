package output

import (
	"context"

	"github.com/hashicorp/terraform-exec/tfexec"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultRunnerFactory creates a real terraform runner using tfexec.
func defaultRunnerFactory(workdir, executable string) (TerraformRunner, error) {
	return tfexec.NewTerraform(workdir, executable)
}

// runInit executes terraform init with appropriate options.
//
//nolint:revive // argument-limit: internal function passing through execution context.
func (e *Executor) runInit(ctx context.Context, runner TerraformRunner, config *ComponentConfig, component, stack string, stderrCapture *quietModeWriter) error {
	defer perf.Track(nil, "output.Executor.runInit")()

	log.Debug("Executing terraform init", "component", component, "stack", stack)

	var initOptions []tfexec.InitOption
	initOptions = append(initOptions, tfexec.Upgrade(false))
	if config.InitRunReconfigure {
		initOptions = append(initOptions, tfexec.Reconfigure(true))
	}

	if err := runner.Init(ctx, initOptions...); err != nil {
		return wrapErrorWithStderr(
			errUtils.Build(errUtils.ErrTerraformInit).WithCause(err).Err(),
			stderrCapture,
		)
	}

	log.Debug("Completed terraform init", "component", component, "stack", stack)
	return nil
}

// runOutput executes terraform output with retry logic.
func (e *Executor) runOutput(ctx context.Context, runner TerraformRunner, component, stack string, stderrCapture *quietModeWriter) (map[string]tfexec.OutputMeta, error) {
	defer perf.Track(nil, "output.Executor.runOutput")()

	log.Debug("Executing terraform output", "component", component, "stack", stack)

	// Add small delay on Windows to prevent file locking issues.
	windowsFileDelay()

	var outputMeta map[string]tfexec.OutputMeta
	err := retryOnWindows(func() error {
		var outputErr error
		outputMeta, outputErr = runner.Output(ctx)
		return outputErr
	})
	if err != nil {
		return nil, wrapErrorWithStderr(err, stderrCapture)
	}

	log.Debug("Completed terraform output", "component", component, "stack", stack)
	return outputMeta, nil
}
