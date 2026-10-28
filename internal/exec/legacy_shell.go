package exec

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/shell"
)

// ShellCommandOption configures subprocess execution.
//
// Deprecated: Use shell.ShellCommandOption.
type (
	ShellCommandOption = shell.ShellCommandOption
	shellCommandConfig = shell.CommandConfig
)

// ExecuteShellSpec configures script execution.
//
// Deprecated: Use shell.ExecuteShellSpec.
type ExecuteShellSpec = shell.ExecuteShellSpec

// ExecuteShellCommand delegates to its owning package.
//
// Deprecated: Use shell.ExecuteShellCommand.
//
//nolint:revive,gocritic // Preserve the legacy public signature; new callers use CommandRequest.
func ExecuteShellCommand(
	atmosConfig schema.AtmosConfiguration,
	command string,
	args []string,
	dir string,
	env []string,
	dryRun bool,
	redirectStdError string,
	opts ...ShellCommandOption,
) error {
	defer perf.Track(nil, "exec.ExecuteShellCommand")()

	return shell.ExecuteCommand(&shell.CommandRequest{AtmosConfig: atmosConfig, Command: command, Args: args, Dir: dir, Env: env, DryRun: dryRun, RedirectStderr: redirectStdError}, opts...)
}

// envVarFromList delegates to its owning package.
//
// Deprecated: Use shell.EnvVarFromList.
func envVarFromList(env []string, key string) string {
	return shell.EnvVarFromList(env, key)
}

// WithInvokingCommand delegates to its owning package.
//
// Deprecated: Use shell.WithInvokingCommand.
func WithInvokingCommand(cmd *cobra.Command) ShellCommandOption {
	defer perf.Track(nil, "exec.WithInvokingCommand")()

	return shell.WithInvokingCommand(cmd)
}

// invokingCommandFromOpts delegates to its owning package.
//
// Deprecated: Use shell.InvokingCommandFromOpts.
func invokingCommandFromOpts(opts ...ShellCommandOption) *cobra.Command {
	return shell.InvokingCommandFromOpts(opts...)
}

// WithExecMetadataParser delegates to its owning package.
//
// Deprecated: Use shell.WithExecMetadataParser.
func WithExecMetadataParser(fn func(subCommand string, exitCode int, output string) any) ShellCommandOption {
	defer perf.Track(nil, "exec.WithExecMetadataParser")()

	return shell.WithExecMetadataParser(fn)
}

// execMetadataParserFromOpts delegates to its owning package.
//
// Deprecated: Use shell.ExecMetadataParserFromOpts.
func execMetadataParserFromOpts(opts ...ShellCommandOption) func(subCommand string, exitCode int, output string) any {
	return shell.ExecMetadataParserFromOpts(opts...)
}

// WithMetricsCallback delegates to its owning package.
//
// Deprecated: Use shell.WithMetricsCallback.
func WithMetricsCallback(fn func(*metricsprocess.ProcessMetrics)) ShellCommandOption {
	defer perf.Track(nil, "exec.WithMetricsCallback")()

	return shell.WithMetricsCallback(fn)
}

// withExecMetadataOutputCapture delegates to its owning package.
//
// Deprecated: Use shell.WithExecMetadataOutputCapture.
func withExecMetadataOutputCapture(stdoutW, stderrW io.Writer) ShellCommandOption {
	return shell.WithExecMetadataOutputCapture(stdoutW, stderrW)
}

// execMetadataOutputCaptureFromOpts delegates to its owning package.
//
// Deprecated: Use shell.ExecMetadataOutputCaptureFromOpts.
func execMetadataOutputCaptureFromOpts(opts ...ShellCommandOption) (io.Writer, io.Writer) {
	return shell.ExecMetadataOutputCaptureFromOpts(opts...)
}

// WithStdoutCapture delegates to its owning package.
//
// Deprecated: Use shell.WithStdoutCapture.
func WithStdoutCapture(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "exec.WithStdoutCapture")()

	return shell.WithStdoutCapture(w)
}

// WithStderrCapture delegates to its owning package.
//
// Deprecated: Use shell.WithStderrCapture.
func WithStderrCapture(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "exec.WithStderrCapture")()

	return shell.WithStderrCapture(w)
}

// WithStdoutOverride delegates to its owning package.
//
// Deprecated: Use shell.WithStdoutOverride.
func WithStdoutOverride(w io.Writer) ShellCommandOption {
	defer perf.Track(nil, "exec.WithStdoutOverride")()

	return shell.WithStdoutOverride(w)
}

// WithProcessStreams delegates to its owning package.
//
// Deprecated: Use shell.WithProcessStreams.
func WithProcessStreams(streams process.Streams) ShellCommandOption {
	defer perf.Track(nil, "exec.WithProcessStreams")()

	return shell.WithProcessStreams(streams)
}

// WithProcessContext delegates to its owning package.
//
// Deprecated: Use shell.WithProcessContext.
func WithProcessContext(ctx context.Context) ShellCommandOption {
	defer perf.Track(nil, "exec.WithProcessContext")()

	return shell.WithProcessContext(ctx)
}

// shellCommandContext delegates to its owning package.
//
// Deprecated: Use shell.ShellCommandContext.
func shellCommandContext(opts ...ShellCommandOption) context.Context {
	return shell.ShellCommandContext(opts...)
}

// shellCommandOutputWriters delegates to its owning package.
//
// Deprecated: Use shell.ShellCommandOutputWriters.
func shellCommandOutputWriters(opts ...ShellCommandOption) provisioner.OutputWriters {
	return shell.ShellCommandOutputWriters(opts...)
}

// WithEnvironment delegates to its owning package.
//
// Deprecated: Use shell.WithEnvironment.
func WithEnvironment(env []string) ShellCommandOption {
	defer perf.Track(nil, "exec.WithEnvironment")()

	return shell.WithEnvironment(env)
}

// ExecuteShell delegates to its owning package.
//
// Deprecated: Use shell.ExecuteShell.
func ExecuteShell(
	command string,
	name string,
	dir string,
	envVars []string,
	dryRun bool,
) error {
	defer perf.Track(nil, "exec.ExecuteShell")()

	return shell.ExecuteShell(command, name, dir, envVars, dryRun)
}

// executeShellMasked delegates to its owning package.
//
// Deprecated: Use shell.ExecuteShellMasked.
func executeShellMasked(spec *ExecuteShellSpec, stdout, stderr, stdoutCapture, stderrCapture io.Writer) error {
	return shell.ExecuteShellMasked(spec, stdout, stderr, stdoutCapture, stderrCapture)
}

// ExecuteShellWithWriters delegates to its owning package.
//
// Deprecated: Use shell.ExecuteShellWithWriters.
func ExecuteShellWithWriters(spec *ExecuteShellSpec) error {
	defer perf.Track(nil, "exec.ExecuteShellWithWriters")()

	return shell.ExecuteShellWithWriters(spec)
}
