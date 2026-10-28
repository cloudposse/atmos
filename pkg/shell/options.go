package shell

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	process "github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

// ShellCommandOption is a functional option for ExecuteShellCommand.
type ShellCommandOption func(*CommandConfig)

// CommandConfig holds optional configuration for shell command execution.
type CommandConfig struct {
	StdoutCapture  io.Writer
	StderrCapture  io.Writer
	StdoutOverride io.Writer
	Streams        *process.Streams
	Ctx            context.Context
	// ProcessEnv replaces os.Environ() as the process environment.
	// When set, ExecuteShellCommand uses this instead of re-reading os.Environ().
	// This is used when auth has already sanitized the environment (e.g., removed IRSA vars).
	ProcessEnv []string
	// InvokingCmd is the Cobra command the user actually invoked (e.g. the
	// terraform plan subcommand). Threaded through so ExecuteTerraform can
	// report the real Flags actually passed (FR-003b) to the exec-metadata
	// sync capture, instead of a pass-through-args collection that cannot
	// contain atmos-recognized flags (research.md Decision 14).
	InvokingCmd *cobra.Command
	// ExecMetadataParser, when set, is called once by captureExecMetadataSync
	// (sync-allowlisted commands only) to obtain command-specific structured
	// data (FR-006) for the execution record. cmd/terraform supplies this —
	// it can safely call pkg/ci/plugins/terraform's output parser, unlike
	// internal/exec, which cannot import it without reintroducing a confirmed
	// import cycle (research.md Decision 18). The output parameter is the
	// scoped exec-metadata output text executeCommandPipeline captured for
	// just the main plan/apply/deploy subprocess (FR-006f).
	ExecMetadataParser func(subCommand string, exitCode int, output string) any

	// ExecMetadataStdoutCapture/ExecMetadataStderrCapture, when set, additionally
	// tee this specific ExecuteShellCommand call's stdout/stderr into these
	// writers, alongside (not instead of) StdoutCapture/StderrCapture. Used
	// internally by executeCommandPipeline (via WithExecMetadataOutputCapture) to
	// scope the exec-metadata parser's input to only the final plan/apply/deploy
	// subprocess invocation, while StdoutCapture/StderrCapture's buffers keep
	// accumulating the whole pipeline's output for other consumers (e.g. CI
	// job-summary hooks) that legitimately need it (FR-006f, research.md
	// Decision 32).
	ExecMetadataStdoutCapture io.Writer
	ExecMetadataStderrCapture io.Writer

	// MetricsCallback, when set, is invoked once after the subprocess exits
	// with the subprocess tree's collected resource-usage metrics (nil-safe:
	// only called when a non-nil result was collected). Lets a caller (e.g.
	// executeMainTerraformCommand) capture this specific shell command's
	// subprocess-tree metrics for combining with atmos's own self-usage, for
	// both the local per-command display and the exec-metadata sync upload.
	MetricsCallback func(*metricsprocess.ProcessMetrics)
}

// WithInvokingCommand provides the Cobra command the user actually invoked,
// so ExecuteTerraform's exec-metadata sync capture can derive Flags from the
// command's own record of explicitly-set flags (proexec.FlagsFromCommand)
// rather than info.AdditionalArgsAndFlags.
func WithInvokingCommand(cmd *cobra.Command) ShellCommandOption {
	defer perf.Track(nil, "shell.WithInvokingCommand")()

	return func(c *CommandConfig) {
		c.InvokingCmd = cmd
	}
}

// InvokingCommandFromOpts extracts the invoking *cobra.Command (if any) set
// via WithInvokingCommand among opts.
func InvokingCommandFromOpts(opts ...ShellCommandOption) *cobra.Command {
	defer perf.Track(nil, "shell.InvokingCommandFromOpts")()

	var cfg CommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.InvokingCmd
}

// WithExecMetadataParser provides a closure that, given the invoking
// subcommand name and the command's own exit code, returns command-specific
// structured data (FR-006) for the execution record — or nil if there is
// none to report. The exitCode parameter is the terraform/tofu subprocess's
// own exit code (research.md Decision 27), supplied by
// captureExecMetadataSync at call time since it isn't known when this
// closure is created. Cmd/terraform
// supplies this closure so ExecuteTerraform's exec-metadata sync capture can
// obtain parsed terraform plan/apply/deploy output without internal/exec
// itself importing the CI plugin's parser (research.md Decision 18).
func WithExecMetadataParser(fn func(subCommand string, exitCode int, output string) any) ShellCommandOption {
	defer perf.Track(nil, "shell.WithExecMetadataParser")()

	return func(c *CommandConfig) {
		c.ExecMetadataParser = fn
	}
}

// ExecMetadataParserFromOpts extracts the parser closure (if any) set via
// WithExecMetadataParser among opts.
func ExecMetadataParserFromOpts(opts ...ShellCommandOption) func(subCommand string, exitCode int, output string) any {
	defer perf.Track(nil, "shell.ExecMetadataParserFromOpts")()

	var cfg CommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.ExecMetadataParser
}

// WithMetricsCallback provides a closure that ExecuteShellCommand invokes
// once, after the subprocess exits, with the subprocess tree's collected
// resource-usage metrics. Used by executeMainTerraformCommand to capture the
// main plan/apply/deploy subprocess's own metrics for combining with atmos's
// self-usage — both for local display and the exec-metadata sync upload.
func WithMetricsCallback(fn func(*metricsprocess.ProcessMetrics)) ShellCommandOption {
	defer perf.Track(nil, "shell.WithMetricsCallback")()

	return func(c *CommandConfig) {
		c.MetricsCallback = fn
	}
}

// WithExecMetadataOutputCapture tees this specific ExecuteShellCommand call's
// stdout/stderr into stdoutW/stderrW, in addition to whatever
// WithStdoutCapture/WithStderrCapture already capture. Used internally by
// executeCommandPipeline to scope the exec-metadata parser's input to only the
// final plan/apply/deploy subprocess invocation (FR-006f), without disturbing
// the broader stdout/stderr buffers other consumers (e.g. cmd/terraform's
// capturedPlanOutput, used by CI job-summary hooks) rely on for the whole
// init+workspace-select+main pipeline. Unexported — an internal
// executeCommandPipeline concern, not part of the ShellCommandOption surface
// cmd/terraform uses.
func WithExecMetadataOutputCapture(stdoutW, stderrW io.Writer) ShellCommandOption {
	defer perf.Track(nil, "shell.WithExecMetadataOutputCapture")()

	return func(c *CommandConfig) {
		c.ExecMetadataStdoutCapture = stdoutW
		c.ExecMetadataStderrCapture = stderrW
	}
}

// ExecMetadataOutputCaptureFromOpts extracts the exec-metadata stdout/stderr
// tee writers (if any) set via WithExecMetadataOutputCapture among opts, so
// callers that retry the underlying invocation (executeShellCommandWithRetry)
// can reset them between attempts.
func ExecMetadataOutputCaptureFromOpts(opts ...ShellCommandOption) (io.Writer, io.Writer) {
	defer perf.Track(nil, "shell.ExecMetadataOutputCaptureFromOpts")()

	var cfg CommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.ExecMetadataStdoutCapture, cfg.ExecMetadataStderrCapture
}

// WithStdoutCapture returns a ShellCommandOption that tees stdout to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStdoutCapture(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "shell.WithStdoutCapture")()

	return func(c *CommandConfig) {
		c.StdoutCapture = w
	}
}

// WithStderrCapture returns a ShellCommandOption that tees stderr to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStderrCapture(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "shell.WithStderrCapture")()

	return func(c *CommandConfig) {
		c.StderrCapture = w
	}
}

// WithStdoutOverride returns a ShellCommandOption that replaces the default stdout
// (os.Stdout) with a different writer. Used to redirect noisy commands (e.g.,
// workspace select) to stderr so they don't pollute data-producing commands like output.
func WithStdoutOverride(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "shell.WithStdoutOverride")()

	return func(c *CommandConfig) {
		c.StdoutOverride = w
	}
}

// WithProcessStreams provides the subprocess standard Streams.
func WithProcessStreams(streams process.Streams) ShellCommandOption {
	defer perf.Track(nil, "shell.WithProcessStreams")()

	return func(c *CommandConfig) {
		c.Streams = &streams
	}
}

// WithProcessContext provides the context used for subprocess execution.
func WithProcessContext(ctx context.Context) ShellCommandOption {
	defer perf.Track(nil, "shell.WithProcessContext")()

	return func(c *CommandConfig) {
		c.Ctx = ctx
	}
}

func ShellCommandContext(opts ...ShellCommandOption) context.Context {
	defer perf.Track(nil, "shell.ShellCommandContext")()

	var cfg CommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Ctx == nil {
		return context.Background()
	}
	return cfg.Ctx
}

func ShellCommandOutputWriters(opts ...ShellCommandOption) provisioner.OutputWriters {
	defer perf.Track(nil, "shell.ShellCommandOutputWriters")()

	var cfg CommandConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Streams == nil {
		return provisioner.OutputWriters{}
	}
	return provisioner.OutputWriters{
		Stdout: cfg.Streams.Stdout,
		Stderr: cfg.Streams.Stderr,
	}
}

// WithEnvironment provides a pre-sanitized process environment for subprocess execution.
// When provided, ExecuteShellCommand uses this instead of re-reading os.Environ().
// Pass nil to fall back to the default os.Environ() behavior.
func WithEnvironment(env []string) ShellCommandOption {
	defer perf.Track(nil, "shell.WithEnvironment")()

	return func(c *CommandConfig) {
		c.ProcessEnv = env
	}
}
