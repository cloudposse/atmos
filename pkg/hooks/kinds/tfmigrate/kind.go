package tfmigrate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

var errMissingHookContext = errors.New("missing tfmigrate hook context")

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

	action, err := tfmigrate.ActionForMode(ctx.Hook.Mode, string(ctx.Event))
	if err != nil {
		return nil, err
	}

	args, err := atmosArgs(ctx, action)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(os.Args[0], args...) // #nosec G204,G702 -- intentional nested Atmos invocation.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "ATMOS_SKIP_HOOKS=*")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tfmigrate hook failed: %w", err)
	}
	return nil, nil
}

func atmosArgs(ctx *hooks.ExecContext, action string) ([]string, error) {
	if ctx == nil || ctx.Hook == nil || ctx.Info == nil {
		return nil, errMissingHookContext
	}
	args := []string{"terraform", "migrate", action}
	args = appendValue(args, ctx.Info.ComponentFromArg)
	args = appendFlagValue(args, "--stack", ctx.Info.Stack)
	args = appendFlagValue(args, "--identity", ctx.Info.Identity)
	args = appendFlagValue(args, "--migration", ctx.Hook.Migration)
	args = appendFlagValue(args, "--tfmigrate-config", ctx.Hook.Config)
	for _, backendConfig := range ctx.Hook.BackendConfig {
		args = appendFlagValue(args, "--backend-config", backendConfig)
	}
	return args, nil
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
