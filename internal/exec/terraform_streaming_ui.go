package exec

// terraform_streaming_ui.go threads the streaming TUI (pkg/terraform/ui) into the
// terraform execution pipeline (terraform_execute_helpers.go,
// terraform_execute_helpers_exec.go), gated by --ui / atmos.yaml / TTY / CI.

import (
	"context"
	"errors"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	tfui "github.com/cloudposse/atmos/pkg/terraform/ui"
	"github.com/cloudposse/atmos/pkg/ui"
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
// ExecuteShellCommand call (e.g. the retry wrapper's stdout/stderr capture); the streaming path
// forwards the same stdout/stderr-capture and exec-metadata-capture writers from shellOpts into
// tfui.ExecuteOptions.StdoutCapture/StderrCapture (see streamingCaptureWriters below), so
// consumers like the retry-condition matcher and smart-init's failure classifier still see the
// terraform/tofu output even when it streamed through the TUI instead of the plain shell path.
// When the component has retry conditions configured, streaming is skipped entirely and the
// shell path always runs: ExecuteShellCommandWithRetry's own capture/reset cycle between
// attempts has no TUI equivalent, so retries always go through the plain shell path.
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

	retryActive := info.ComponentRetrySection != nil && len(info.ComponentRetrySection.Conditions) > 0
	if retryActive {
		return runShell()
	}

	if !tfui.ShouldUseStreamingUI(info.UIFlagExplicitlySet, info.UIEnabled, atmosConfig.Components.Terraform.UI.Enabled, req.gatePhase) {
		// Warn only when the user actually asked for streaming (flag/config) on a subcommand
		// that doesn't support it (e.g. refresh) - not for the ordinary unrequested/CI/no-TTY
		// cases, which should stay silent.
		if tfui.UIRequestedButUnsupported(info.UIFlagExplicitlySet, info.UIEnabled, atmosConfig.Components.Terraform.UI.Enabled, req.gatePhase) {
			ui.Warning(fmt.Sprintf("Streaming UI (--ui) is not supported for 'terraform %s'; using standard output instead", req.subCommand))
		}
		return runShell()
	}

	stdoutCapture, stderrCapture := streamingCaptureWriters(req.shellOpts)
	execOpts := &tfui.ExecuteOptions{
		Command:       info.Command,
		Args:          req.args,
		WorkingDir:    req.componentPath,
		Env:           info.ComponentEnvList,
		Component:     info.FinalComponent,
		Stack:         info.Stack,
		SubCommand:    req.subCommand,
		Workspace:     req.workspace,
		DryRun:        info.DryRun,
		RenderConfig:  tfui.BuildRenderConfig(atmosConfig.Components.Terraform.UI),
		StdoutCapture: stdoutCapture,
		StderrCapture: stderrCapture,
	}

	ctx := shellCommandContext(req.shellOpts...)
	err := dispatchStreamingExecutor(ctx, req.subCommand, info.DryRun, execOpts)
	if errors.Is(err, errUtils.ErrStreamingNotSupported) {
		log.Debug("Streaming UI not supported, falling back to regular execution")
		return runShell()
	}
	return err
}

// streamingCaptureWriters builds a shellCommandConfig from shellOpts (applying the option funcs
// to a zero config, mirroring execMetadataOutputCaptureFromOpts) and returns the stdout/stderr
// writers the streaming TUI executors should additionally tee their output into: the ordinary
// WithStdoutCapture/WithStderrCapture writer combined with the scoped exec-metadata tee writer
// (withExecMetadataOutputCapture) via io.MultiWriter when both are set, so neither consumer's
// capture goes dark just because a phase happened to run through the TUI instead of the plain
// shell path. Returns (nil, nil) when shellOpts requests no capture at all.
func streamingCaptureWriters(shellOpts []ShellCommandOption) (stdout, stderr io.Writer) {
	var cfg shellCommandConfig
	for _, opt := range shellOpts {
		opt(&cfg)
	}
	return combineCaptureWriters(cfg.stdoutCapture, cfg.execMetadataStdoutCapture),
		combineCaptureWriters(cfg.stderrCapture, cfg.execMetadataStderrCapture)
}

// combineCaptureWriters returns a over io.MultiWriter(a, b) when both are set, whichever of a/b
// is non-nil when only one is, or nil when neither is set.
func combineCaptureWriters(a, b io.Writer) io.Writer {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return io.MultiWriter(a, b)
	}
}

// streamingExecutorFunc is the shared signature of tfui.Execute and every
// tfui.Execute<Phase> variant, letting selectStreamingExecutor hand one back
// without invoking it.
type streamingExecutorFunc func(context.Context, *tfui.ExecuteOptions) error

// selectStreamingExecutor resolves the tfui.Execute* variant matching
// subCommand without invoking it. Both dispatchStreamingExecutor and its
// tests go through this single routing table, so a test can assert on the
// returned function's identity (e.g. that "providers-lock" resolves to
// tfui.ExecuteInit specifically) instead of only observing an error both
// ExecuteInit and the plain Execute fallback would produce identically
// outside a supported interactive environment.
//
// Workspace select/new and the after-init providers-lock hook share the init
// spinner (ExecuteInit) since neither has a dedicated TUI phase of its own. Dry
// runs always use the plain Execute path, which short-circuits without touching
// the terminal.
func selectStreamingExecutor(subCommand string, dryRun bool) streamingExecutorFunc {
	if !dryRun {
		switch subCommand {
		case subcommandApply:
			return tfui.ExecuteApply
		case "destroy":
			return tfui.ExecuteDestroy
		case "plan":
			return tfui.ExecutePlan
		case subcommandInit, subcommandWorkspace, subcommandProvidersLock:
			return tfui.ExecuteInit
		}
	}
	return tfui.Execute
}

// dispatchStreamingExecutor routes to the tfui.Execute* variant matching subCommand.
// The caller's cancellation (e.g. a shell-option deadline) flows through ctx, so a
// cancelled/timed-out caller can stop a running streaming Terraform process instead
// of leaving it orphaned.
func dispatchStreamingExecutor(ctx context.Context, subCommand string, dryRun bool, execOpts *tfui.ExecuteOptions) error {
	return selectStreamingExecutor(subCommand, dryRun)(ctx, execOpts)
}
