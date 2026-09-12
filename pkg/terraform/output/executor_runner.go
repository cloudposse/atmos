package output

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-exec/tfexec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
)

// defaultRunnerFactory creates a real terraform runner using tfexec.
func defaultRunnerFactory(workdir, executable string) (TerraformRunner, error) {
	return tfexec.NewTerraform(workdir, executable)
}

// runInit executes terraform init with the given reconfigure/upgrade flags. If
// the init subcommand itself fails with a diagnostic that terraform/tofu
// recognizes as requiring `-upgrade` or `-reconfigure` (e.g. Atmos's own decision
// under-estimated what init needed), it consults autoinit.ShouldRecover and
// retries once with the flag(s) the diagnostic calls for; a policy error (the
// caller opted out of the flag init now says it needs) is joined with the
// original init error and returned.
//
//nolint:revive // argument-limit: internal function passing through execution context.
func (e *Executor) runInit(
	ctx context.Context,
	runner TerraformRunner,
	config *ComponentConfig,
	component, stack string,
	stderrCapture *quietModeWriter,
	pluginCache tfplugin.Cache,
	reconfigure, upgrade bool,
) error {
	defer perf.Track(nil, "output.Executor.runInit")()

	run := func() error {
		return e.runInitOnce(ctx, runner, config, component, stack, stderrCapture, reconfigure, upgrade)
	}

	if pluginCache.Directory == "" {
		return run()
	}

	var initErr error
	if err := filelock.New(pluginCache.InitLockPathForWorkdir(config.ComponentPath)).WithExclusive(ctx, func() error {
		initErr = run()
		return nil
	}); err != nil {
		return errUtils.Build(errUtils.ErrTerraformInit).
			WithCause(fmt.Errorf("lock provider plugin cache: %w", err)).
			Err()
	}
	return initErr
}

// runInitOnce runs `terraform init` once with reconfigure/upgrade, and — on
// failure — attempts exactly one recovery retry when terraform/tofu's own
// diagnostic says the init it just ran needed `-upgrade` or `-reconfigure`
// (Atmos's own smart-init decision under-estimated what init would need).
//
//nolint:revive // argument-limit: internal function passing through execution context.
func (e *Executor) runInitOnce(
	ctx context.Context,
	runner TerraformRunner,
	config *ComponentConfig,
	component, stack string,
	stderrCapture *quietModeWriter,
	reconfigure, upgrade bool,
) error {
	log.Debug("Executing terraform init", "component", component, "stack", stack, "reconfigure", reconfigure, "upgrade", upgrade)

	// Reset before this attempt: stderrCapture is shared across every terraform-exec call in
	// this execute() invocation, and diagnosticText below classifies this init's own failure --
	// it must not see leftover stderr from an earlier, already-handled command (e.g. a prior
	// successful init/workspace-select, or a prior failed `output` this init is recovering from).
	stderrCapture.Reset()
	err := runner.Init(ctx, buildInitOptions(reconfigure, upgrade)...)
	if err == nil {
		log.Debug("Completed terraform init", "component", component, "stack", stack)
		return nil
	}

	diag := autoinit.Classify(diagnosticText(err, stderrCapture))
	if !diag.UpgradeRequired && !diag.ReconfigureRequired {
		return wrapErrorWithStderr(errUtils.Build(errUtils.ErrTerraformInit).WithCause(err).Err(), stderrCapture)
	}

	rec, policyErr := autoinit.ShouldRecover(diag, config.InitMode, config.InitReconfigure, config.InitUpgrade, false)
	if policyErr != nil {
		return wrapErrorWithStderr(errUtils.Build(errUtils.ErrTerraformInit).WithCause(errors.Join(err, policyErr)).Err(), stderrCapture)
	}
	if !rec.Run {
		return wrapErrorWithStderr(errUtils.Build(errUtils.ErrTerraformInit).WithCause(err).Err(), stderrCapture)
	}

	log.Warn("autoinit: terraform init reported it needs more flags, retrying",
		"component", component, "stack", stack, "matched", diag.Matched,
		"reconfigure", reconfigure || rec.WithReconfigure, "upgrade", upgrade || rec.WithUpgrade)

	retryErr := runner.Init(ctx, buildInitOptions(reconfigure || rec.WithReconfigure, upgrade || rec.WithUpgrade)...)
	if retryErr != nil {
		return wrapErrorWithStderr(errUtils.Build(errUtils.ErrTerraformInit).WithCause(retryErr).Err(), stderrCapture)
	}

	log.Debug("Completed terraform init after recovery retry", "component", component, "stack", stack)
	return nil
}

// buildInitOptions renders the tfexec.InitOption slice for reconfigure/upgrade.
func buildInitOptions(reconfigure, upgrade bool) []tfexec.InitOption {
	options := []tfexec.InitOption{tfexec.Upgrade(upgrade)}
	if reconfigure {
		options = append(options, tfexec.Reconfigure(true))
	}
	return options
}

// runOutput executes terraform output with retry logic.
func (e *Executor) runOutput(ctx context.Context, runner TerraformRunner, component, stack string, stderrCapture *quietModeWriter) (map[string]tfexec.OutputMeta, error) {
	defer perf.Track(nil, "output.Executor.runOutput")()

	log.Debug("Executing terraform output", "component", component, "stack", stack)

	// Add small delay on Windows to prevent file locking issues.
	windowsFileDelay()

	// Reset before this attempt: see runInitOnce's comment on stderrCapture.Reset() -- a failed
	// output's diagnosticText (used by runOutputWithInitRecovery's Classify call) must reflect
	// only this output call's own stderr, not leftover output from the init/workspace-select
	// calls that already succeeded earlier in this same execute() invocation.
	stderrCapture.Reset()

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
