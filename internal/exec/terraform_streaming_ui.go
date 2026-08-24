package exec

// terraform_streaming_ui.go threads the streaming TUI (pkg/terraform/ui) into the
// terraform execution pipeline (terraform_execute_helpers.go,
// terraform_execute_helpers_exec.go), gated by --ui / atmos.yaml / TTY / CI.

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	tfui "github.com/cloudposse/atmos/pkg/terraform/ui"
)

// streamingExecRequest bundles the arguments executeStreamingOrShell needs beyond
// atmosConfig/info, keeping the function's own argument count within lint limits.
type streamingExecRequest struct {
	componentPath  string
	args           []string
	gatePhase      string
	subCommand     string
	workspace      string
	redirectStdErr string
	shellOpts      []ShellCommandOption
}

// executeStreamingOrShell runs a terraform phase through the streaming TUI when enabled
// (per tfui.ShouldUseStreamingUI(req.gatePhase)), falling back to a plain ExecuteShellCommand
// when streaming is disabled or unsupported (non-TTY, CI, or the TUI reports
// ErrStreamingNotSupported). The request's shellOpts are forwarded to every
// ExecuteShellCommand call (e.g. the retry wrapper's stdout/stderr capture) — the TUI
// executors have no equivalent, so they run without them.
//
// The request's gatePhase and subCommand are usually the same value, except for
// workspace select/new: those gate on the "init" phase (workspace setup is part of the
// init lifecycle and has no dedicated ShouldUseStreamingUI case) while still labelling
// the TUI dispatch "workspace".
func executeStreamingOrShell(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, req *streamingExecRequest) error {
	runShell := func() error {
		return ExecuteShellCommand(
			*atmosConfig,
			info.Command,
			req.args,
			req.componentPath,
			info.ComponentEnvList,
			info.DryRun,
			req.redirectStdErr,
			req.shellOpts...,
		)
	}

	if !tfui.ShouldUseStreamingUI(info.UIFlagExplicitlySet, info.UIEnabled, atmosConfig.Components.Terraform.UI.Enabled, req.gatePhase) {
		return runShell()
	}

	execOpts := &tfui.ExecuteOptions{
		Command:    info.Command,
		Args:       req.args,
		WorkingDir: req.componentPath,
		Env:        info.ComponentEnvList,
		Component:  info.FinalComponent,
		Stack:      info.Stack,
		SubCommand: req.subCommand,
		Workspace:  req.workspace,
		DryRun:     info.DryRun,
	}

	err := dispatchStreamingExecutor(req.subCommand, info.DryRun, execOpts)
	if errors.Is(err, errUtils.ErrStreamingNotSupported) {
		log.Debug("Streaming UI not supported, falling back to regular execution")
		return runShell()
	}
	return err
}

// dispatchStreamingExecutor routes to the tfui.Execute* variant matching subCommand.
// Workspace select/new share the init spinner (ExecuteInit) since they have no
// dedicated TUI phase of their own. Dry runs always use the plain Execute path, which
// short-circuits without touching the terminal.
func dispatchStreamingExecutor(subCommand string, dryRun bool, execOpts *tfui.ExecuteOptions) error {
	if !dryRun {
		switch subCommand {
		case subcommandApply:
			return tfui.ExecuteApply(context.Background(), execOpts)
		case "destroy":
			return tfui.ExecuteDestroy(context.Background(), execOpts)
		case "plan":
			return tfui.ExecutePlan(context.Background(), execOpts)
		case subcommandInit, subcommandWorkspace:
			return tfui.ExecuteInit(context.Background(), execOpts)
		}
	}
	return tfui.Execute(context.Background(), execOpts)
}
