package tfmigrate

import (
	"fmt"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

var errMissingHookContext = fmt.Errorf("%w: tfmigrate hook context", errUtils.ErrNilInput)

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:      "tfmigrate",
		Command:   tfmigrate.Command,
		Engine:    &Engine{},
		OnFailure: hooks.OnFailureFail,
	}); err != nil {
		panic("failed to register built-in tfmigrate kind: " + err.Error())
	}
}

// Engine adapts hook configuration to a tfmigrate command invocation.
type Engine struct{}

// Run resolves the hook mode from the current lifecycle event and delegates
// execution to `atmos terraform migrate`. Using the Atmos subcommand keeps
// hook execution on the same source/workdir-compatible path as explicit CLI
// migration runs.
func (e *Engine) Run(ctx *hooks.ExecContext) (*hooks.Output, error) {
	defer perf.Track(nil, "hooks.tfmigrate.Engine.Run")()

	if ctx == nil || ctx.Hook == nil || ctx.Info == nil {
		return nil, errMissingHookContext
	}

	action, err := tfmigrate.ActionForMode(ctx.Hook.Mode, string(ctx.Event))
	if err != nil {
		return nil, err
	}

	args := atmosArgs(ctx, action)
	// Use os.Executable() to get the absolute path to the currently running binary.
	// os.Args[0] can be a relative path (e.g. ./build/atmos), which breaks once the
	// process working directory differs from the invocation directory (--chdir).
	atmosBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("tfmigrate hook failed to determine atmos executable path: %w", err)
	}
	cmd := exec.Command(atmosBin, args...) // #nosec G204,G702 -- intentional nested Atmos invocation.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "ATMOS_SKIP_HOOKS=*")
	if err := cmd.Run(); err != nil {
		// ApplyOnFailure resolves ctx.Hook.OnFailure ("warn"/"ignore"/"fail",
		// same as the command/tflint/checkov kinds honor via CommandEngine)
		// instead of always hard-failing regardless of what the hook config says.
		return nil, hooks.ApplyOnFailure(ctx, fmt.Errorf("tfmigrate hook failed: %w", err))
	}
	return nil, nil
}

// atmosArgs assumes Run has already validated the context (non-nil ctx, Hook, and Info).
func atmosArgs(ctx *hooks.ExecContext, action string) []string {
	args := []string{"terraform", "migrate", action}
	args = appendValue(args, ctx.Info.ComponentFromArg)
	args = appendFlagValue(args, "--stack", ctx.Info.Stack)
	args = appendFlagValue(args, "--identity", ctx.Info.Identity)
	args = appendFlagValue(args, "--migration", ctx.Hook.Migration)
	args = appendFlagValue(args, "--tfmigrate-config", ctx.Hook.Config)
	for _, backendConfig := range ctx.Hook.BackendConfig {
		args = appendFlagValue(args, "--backend-config", backendConfig)
	}
	if ctx.Info.DryRun {
		// The nested `atmos terraform migrate` invocation already no-ops
		// correctly under --dry-run (skips EnsureResolved/EnsureLocalHistoryDir
		// and prints instead of executing) - forward the flag rather than
		// letting a dry-run outer command silently run a real migration.
		args = append(args, "--dry-run")
	}
	return args
}

func appendValue(args []string, value string) []string {
	if value == "" {
		return args
	}
	return append(args, value)
}

func appendFlagValue(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}
