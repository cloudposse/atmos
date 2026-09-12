package output

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
	"github.com/cloudposse/atmos/pkg/version"
)

// tfVarEnvPrefix is the environment variable prefix Terraform/OpenTofu use for
// variable values (see addTerraformVarsToEnv in environment.go).
const tfVarEnvPrefix = "TF_VAR_"

// ensureInitialized implements Atmos's "smart init" policy for a single terraform
// output execution: it decides -- via autoinit.Decide -- whether `terraform init`
// needs to run at all, and with which flags, then always ensures the workspace is
// selected afterward (workspace select needs an initialized backend, which the
// decision guarantees).
//
// When skipInit is set (the GetOutputSkipInit path, used by after-terraform-apply
// hooks), this is a deliberate no-op: the caller has already established that
// .terraform/ state is correct in-process, and neither init nor workspace
// selection should run here. This matches execute()'s pre-autoinit behavior,
// where both were gated by the same `if !skipInit` block.
//
//nolint:revive // argument-limit: orchestration step wiring init through several execute() collaborators.
func (e *Executor) ensureInitialized(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	runner TerraformRunner,
	config *ComponentConfig,
	component, stack string,
	stderrCapture *quietModeWriter,
	pluginCache tfplugin.Cache,
	environMap map[string]string,
	skipInit bool,
) error {
	defer perf.Track(atmosConfig, "output.Executor.ensureInitialized")()

	if skipInit {
		return nil
	}

	workspaceMgr := &defaultWorkspaceManager{}
	in := buildAutoInitInputs(config, environMap)
	decision := autoinit.Decide(&autoinit.Request{
		Mode:        config.InitMode,
		Reconfigure: config.InitReconfigure,
		Upgrade:     config.InitUpgrade,
		Force:       config.WorkdirReprovisioned,
		Inputs:      in,
	})

	if decision.RunInit {
		workspaceMgr.CleanWorkspace(atmosConfig, config.ComponentPath)
		if err := e.runInit(ctx, runner, config, component, stack, stderrCapture, pluginCache, decision.Reconfigure, decision.Upgrade); err != nil {
			return err
		}
		if err := autoinit.Record(in, initArgsForLog(decision.Reconfigure, decision.Upgrade), version.Version); err != nil {
			log.Debug("autoinit: failed to record init marker", "component", component, "stack", stack, "error", err)
		}
	} else {
		log.Debug("autoinit: skipping terraform init", "component", component, "stack", stack, "reason", string(decision.Reason))
	}

	return workspaceMgr.EnsureWorkspace(ctx, runner, config.Workspace, config.BackendType, component, stack, stderrCapture)
}

// runOutputWithInitRecovery runs `terraform output` and, if it fails with a
// diagnostic that terraform/tofu itself recognizes as an init problem (e.g. a
// stale smart-init skip, or a backend/upgrade requirement discovered only once
// output actually touches the backend), recovers by re-running init with the
// flag(s) the diagnostic calls for and retrying output exactly once.
//
//nolint:revive // argument-limit: recovery orchestration needs the same collaborators as ensureInitialized.
func (e *Executor) runOutputWithInitRecovery(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	runner TerraformRunner,
	config *ComponentConfig,
	component, stack string,
	stderrCapture *quietModeWriter,
	pluginCache tfplugin.Cache,
	environMap map[string]string,
	skipInit bool,
) (map[string]tfexec.OutputMeta, error) {
	defer perf.Track(atmosConfig, "output.Executor.runOutputWithInitRecovery")()

	outputMeta, err := e.runOutput(ctx, runner, component, stack, stderrCapture)
	if err == nil {
		return outputMeta, nil
	}

	diag := autoinit.Classify(diagnosticText(err, stderrCapture))
	optedOut := skipInit || config.InitMode == schema.TerraformInitModeNever
	rec, policyErr := autoinit.ShouldRecover(diag, config.InitMode, config.InitReconfigure, config.InitUpgrade, optedOut)
	if policyErr != nil {
		return nil, errors.Join(err, policyErr)
	}
	if !rec.Run {
		return nil, err
	}

	log.Warn(
		"autoinit: terraform output failed with an init diagnostic, recovering",
		"component", component, "stack", stack, "matched", diag.Matched,
		"reconfigure", rec.WithReconfigure, "upgrade", rec.WithUpgrade,
	)

	workspaceMgr := &defaultWorkspaceManager{}
	workspaceMgr.CleanWorkspace(atmosConfig, config.ComponentPath)
	if err := e.runInit(ctx, runner, config, component, stack, stderrCapture, pluginCache, rec.WithReconfigure, rec.WithUpgrade); err != nil {
		return nil, err
	}
	if err := workspaceMgr.EnsureWorkspace(ctx, runner, config.Workspace, config.BackendType, component, stack, stderrCapture); err != nil {
		return nil, err
	}

	in := buildAutoInitInputs(config, environMap)
	if recordErr := autoinit.Record(in, initArgsForLog(rec.WithReconfigure, rec.WithUpgrade), version.Version); recordErr != nil {
		log.Debug("autoinit: failed to record init marker after recovery", "component", component, "stack", stack, "error", recordErr)
	}

	return e.runOutput(ctx, runner, component, stack, stderrCapture)
}

// buildAutoInitInputs assembles autoinit.Inputs from config and the subprocess
// environment map that was (or will be) passed to the terraform runner via
// SetEnv, so the fingerprint reflects the exact environment terraform/tofu sees.
func buildAutoInitInputs(config *ComponentConfig, environMap map[string]string) *autoinit.Inputs {
	in := &autoinit.Inputs{
		ComponentPath: config.ComponentPath,
		PassVars:      config.PassVars,
		Binary:        config.Executable,
		// The map's own "comma ok" lookup distinguishes an explicit override (e.g. a component
		// `env:` entry that clears an inherited value to "") from the key simply not being in
		// environMap at all; a miss falls back to os.Getenv inside autoinit itself (see
		// Inputs.EnvLookup's doc comment), so there is no need to duplicate that fallback here.
		// This must reflect exactly what runner.SetEnv(environMap) hands to the terraform/tofu
		// subprocess -- otherwise the fingerprint can record a stale inherited value while
		// terraform actually receives an explicit empty override, and smart init would then
		// wrongly skip a required re-init.
		EnvLookup: func(key string) (string, bool) {
			v, ok := environMap[key]
			return v, ok
		},
	}
	if config.PassVars {
		in.Extra = tfVarExtras(environMap)
	}
	return in
}

// tfVarExtras collects the TF_VAR_* entries from environMap so they participate
// in the init fingerprint (see issue #1412 -- these vars can affect what
// `terraform init` resolves, e.g. a module version pinned to var.foo).
func tfVarExtras(environMap map[string]string) map[string]string {
	extra := make(map[string]string, len(environMap))
	for k, v := range environMap {
		if strings.HasPrefix(k, tfVarEnvPrefix) {
			extra[k] = v
		}
	}
	return extra
}

// diagnosticText concatenates an error's message with any captured stderr so
// autoinit.Classify can inspect the full terraform/tofu diagnostic output, not
// just the (often generic) Go error message.
func diagnosticText(err error, stderrCapture *quietModeWriter) string {
	text := err.Error()
	if stderrCapture != nil {
		text += stderrCapture.String()
	}
	return text
}

// initArgsForLog renders the extra flags an init ran with, for the marker's
// InitArgs field (troubleshooting/debug purposes only).
func initArgsForLog(reconfigure, upgrade bool) []string {
	var args []string
	if reconfigure {
		args = append(args, "-reconfigure")
	}
	if upgrade {
		args = append(args, "-upgrade")
	}
	return args
}
