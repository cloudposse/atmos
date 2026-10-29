package component

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

// ExecutionOption is a functional option for ExecuteShellCommand.
type ExecutionOption func(*ExecutionOptions)

// ExecutionOptions holds optional configuration for shell command execution.
type ExecutionOptions struct {
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
	// internally by executeCommandPipeline (via WithMetadataOutputCapture) to
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
func WithInvokingCommand(cmd *cobra.Command) ExecutionOption {
	defer perf.Track(nil, "component.WithInvokingCommand")()

	return func(c *ExecutionOptions) {
		c.InvokingCmd = cmd
	}
}

// InvokingCommandFromOptions extracts the invoking *cobra.Command (if any) set
// via WithInvokingCommand among opts.
func InvokingCommandFromOptions(opts ...ExecutionOption) *cobra.Command {
	defer perf.Track(nil, "component.InvokingCommandFromOptions")()

	var cfg ExecutionOptions
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
func WithExecMetadataParser(fn func(subCommand string, exitCode int, output string) any) ExecutionOption {
	defer perf.Track(nil, "component.WithExecMetadataParser")()

	return func(c *ExecutionOptions) {
		c.ExecMetadataParser = fn
	}
}

// MetadataParserFromOptions extracts the parser closure (if any) set via
// WithExecMetadataParser among opts.
func MetadataParserFromOptions(opts ...ExecutionOption) func(subCommand string, exitCode int, output string) any {
	defer perf.Track(nil, "component.MetadataParserFromOptions")()

	var cfg ExecutionOptions
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
func WithMetricsCallback(fn func(*metricsprocess.ProcessMetrics)) ExecutionOption {
	defer perf.Track(nil, "component.WithMetricsCallback")()

	return func(c *ExecutionOptions) {
		c.MetricsCallback = fn
	}
}

// WithMetadataOutputCapture tees this specific ExecuteShellCommand call's
// stdout/stderr into stdoutW/stderrW, in addition to whatever
// WithStdoutCapture/WithStderrCapture already capture. Used internally by
// executeCommandPipeline to scope the exec-metadata parser's input to only the
// final plan/apply/deploy subprocess invocation (FR-006f), without disturbing
// the broader stdout/stderr buffers other consumers (e.g. cmd/terraform's
// capturedPlanOutput, used by CI job-summary hooks) rely on for the whole
// init+workspace-select+main pipeline.
func WithMetadataOutputCapture(stdoutW, stderrW io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithMetadataOutputCapture")()

	return func(c *ExecutionOptions) {
		c.ExecMetadataStdoutCapture = stdoutW
		c.ExecMetadataStderrCapture = stderrW
	}
}

// MetadataOutputCaptureFromOptions extracts the exec-metadata stdout/stderr
// tee writers (if any) set via WithMetadataOutputCapture among opts, so
// callers that retry the underlying invocation (executeShellCommandWithRetry)
// can reset them between attempts.
func MetadataOutputCaptureFromOptions(opts ...ExecutionOption) (io.Writer, io.Writer) {
	defer perf.Track(nil, "component.MetadataOutputCaptureFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.ExecMetadataStdoutCapture, cfg.ExecMetadataStderrCapture
}

// WithStdoutCapture returns an ExecutionOption that tees stdout to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStdoutCapture(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStdoutCapture")()

	return func(c *ExecutionOptions) {
		c.StdoutCapture = w
	}
}

// WithStderrCapture returns an ExecutionOption that tees stderr to the provided writer.
// The captured output includes secret masking (post-MaskWriter).
func WithStderrCapture(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStderrCapture")()

	return func(c *ExecutionOptions) {
		c.StderrCapture = w
	}
}

// WithStdoutOverride returns an ExecutionOption that replaces the default stdout
// (os.Stdout) with a different writer. Used to redirect noisy commands (e.g.,
// workspace select) to stderr so they don't pollute data-producing commands like output.
func WithStdoutOverride(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStdoutOverride")()

	return func(c *ExecutionOptions) {
		c.StdoutOverride = w
	}
}

// WithProcessStreams provides the subprocess standard streams.
func WithProcessStreams(streams process.Streams) ExecutionOption {
	defer perf.Track(nil, "component.WithProcessStreams")()

	return func(c *ExecutionOptions) {
		c.Streams = &streams
	}
}

// WithProcessContext provides the context used for subprocess execution.
func WithProcessContext(ctx context.Context) ExecutionOption {
	defer perf.Track(nil, "component.WithProcessContext")()

	return func(c *ExecutionOptions) {
		c.Ctx = ctx
	}
}

// ContextFromOptions returns the last supplied context or a background fallback.
func ContextFromOptions(opts ...ExecutionOption) context.Context {
	defer perf.Track(nil, "component.ContextFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Ctx == nil {
		return context.Background()
	}
	return cfg.Ctx
}

// WithEnvironment provides a pre-sanitized process environment for subprocess execution.
// When provided, ExecuteShellCommand uses this instead of re-reading os.Environ().
// Pass nil to fall back to the default os.Environ() behavior.
func WithEnvironment(env []string) ExecutionOption {
	defer perf.Track(nil, "component.WithEnvironment")()

	return func(c *ExecutionOptions) {
		c.ProcessEnv = env
	}
}
