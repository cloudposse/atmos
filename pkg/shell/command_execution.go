package shell

import (
	"context"
	"io"
	"os"
	"runtime"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/diagnostics"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/process"
	terminalpkg "github.com/cloudposse/atmos/pkg/terminal"
)

const redirectedFileMode = 0o644

// commandExecution owns the streams and diagnostics of one subprocess invocation.
// Captures belong to the caller; only redirect files and paced writers are closed.
type commandExecution struct {
	request      *CommandRequest
	config       *CommandConfig
	streams      process.Streams
	diagnostic   diagnostics.Config
	diagnosticID string
	startedAt    time.Time
	pacedClosers []io.Closer
	maskers      []*ioLayer.StreamingMaskWriter
	stdoutTarget io.Writer
	stdout       io.Writer
	stderr       io.Writer
	redirect     string
	redirectFile *os.File
}

func newCommandExecution(request *CommandRequest, config *CommandConfig) *commandExecution {
	return &commandExecution{
		request:      request,
		config:       config,
		streams:      commandStreams(config),
		diagnostic:   diagnostics.FromSchema(request.AtmosConfig.Diagnostics),
		diagnosticID: diagnostics.NewID("process"),
		startedAt:    time.Now(),
		redirect:     request.RedirectStderr,
	}
}

func commandStreams(config *CommandConfig) process.Streams {
	if config.Streams == nil {
		return process.OSStreams()
	}
	streams := *config.Streams
	if streams.Stdin == nil {
		streams.Stdin = os.Stdin
	}
	if streams.Stdout == nil {
		streams.Stdout = os.Stdout
	}
	if streams.Stderr == nil {
		streams.Stderr = os.Stderr
	}
	return streams
}

func (e *commandExecution) prepareOutput() error {
	e.prepareStdout()
	if runtime.GOOS == "windows" && e.redirect == "/dev/null" {
		e.redirect = "NUL"
	}
	if e.redirect == "/dev/stdout" {
		e.stdout = &synchronizedWriter{w: e.stdout}
	}
	switch e.redirect {
	case "", "/dev/stderr":
		e.stderr = e.standardStderr()
	case "/dev/stdout":
		e.stderr = e.mergedStderr()
	default:
		file, err := os.OpenFile(e.redirect, os.O_WRONLY|os.O_CREATE, redirectedFileMode)
		if err != nil {
			log.Warn(err.Error())
			return err
		}
		e.redirectFile = file
		e.stderr = e.newMasked(file)
	}
	return nil
}

// newMasked holds possible secret prefixes between writes so a secret split
// across pipe reads is still masked. Interactive runs keep prompts visible by
// not holding unfinished lines for regex patterns. Maskers are flushed on exit.
func (e *commandExecution) newMasked(writer io.Writer) io.Writer {
	masker := ioLayer.NewStreamingMaskWriter(writer, ioLayer.MaskOptionsForStdin(e.streams.Stdin)...)
	e.maskers = append(e.maskers, masker)
	return masker
}

// prepareStdout tees masked terminal output alongside caller-owned captures.
// StdoutOverride redirects noisy commands (e.g., workspace select) away from
// data-producing stdout. Captures retain the script's raw output as before.
func (e *commandExecution) prepareStdout() {
	e.stdoutTarget = e.streams.Stdout
	if e.config.StdoutOverride != nil {
		e.stdoutTarget = e.config.StdoutOverride
	}
	e.stdoutTarget = maybePaceTerminalWriter(e.stdoutTarget, e.request.AtmosConfig.Settings.Terminal.Speed, &e.pacedClosers)
	writers := []io.Writer{e.newMasked(e.stdoutTarget)}
	writers = e.withDiagnostics(writers, "stdout")
	writers = appendCaptures(writers, e.config.StdoutCapture, e.config.ExecMetadataStdoutCapture)
	e.stdout = io.MultiWriter(writers...)
}

func (e *commandExecution) standardStderr() io.Writer {
	target := maybePaceTerminalWriter(e.streams.Stderr, e.request.AtmosConfig.Settings.Terminal.Speed, &e.pacedClosers)
	writers := []io.Writer{e.newMasked(target)}
	writers = e.withDiagnostics(writers, "stderr")
	writers = appendCaptures(writers, e.config.StderrCapture, e.config.ExecMetadataStderrCapture)
	return io.MultiWriter(writers...)
}

func (e *commandExecution) mergedStderr() io.Writer {
	masked := e.newMasked(e.stdout)
	// ExecMetadataStderrCapture is intentionally omitted. Stdout already includes
	// ExecMetadataStdoutCapture, and the masker writes through stdout. Adding the
	// stderr metadata writer would duplicate content in combineExecMetadataOutput.
	if e.config.StderrCapture != nil {
		return io.MultiWriter(masked, e.config.StderrCapture)
	}
	return masked
}

func (e *commandExecution) withDiagnostics(writers []io.Writer, stream string) []io.Writer {
	if diagnostics.OutputEnabled(e.diagnostic) {
		writers = append(writers, diagnostics.NewOutputWriter(e.diagnostic, e.diagnosticID, stream))
	}
	return writers
}

func appendCaptures(writers []io.Writer, captures ...io.Writer) []io.Writer {
	for _, capture := range captures {
		if capture != nil {
			writers = append(writers, capture)
		}
	}
	return writers
}

func (e *commandExecution) closeRedirectFile() {
	if e.redirectFile == nil {
		return
	}
	if err := e.redirectFile.Close(); err != nil {
		log.Warn(err.Error())
	}
}

func (e *commandExecution) emitStart() {
	log.Debug("Executing", "command", e.request.Command, "args", e.request.Args)
	emitDiagnosticsEvent(e.diagnostic, &diagnostics.Event{
		Type: "process.start", ID: e.diagnosticID,
		Command: e.request.Command, Args: append([]string{}, e.request.Args...),
		CWD: resolveDiagnosticsCWD(e.request.Dir), DryRun: diagnostics.Bool(e.request.DryRun),
		TTY:            diagnostics.Bool(streamsHaveTTY(e.streams)),
		StdinTTY:       diagnostics.Bool(isTTYReader(e.streams.Stdin)),
		StdoutTTY:      diagnostics.Bool(terminalpkg.IsTTYWriter(e.stdoutTarget)),
		StderrTTY:      diagnostics.Bool(terminalpkg.IsTTYWriter(e.streams.Stderr)),
		RedirectStderr: e.redirect,
	})
}

func (e *commandExecution) finishDryRun() error {
	err := closePacedWriters(e.pacedClosers)
	emitDiagnosticsEvent(e.diagnostic, &diagnostics.Event{
		Type: "process.end", ID: e.diagnosticID, Started: diagnostics.Bool(false),
		Success: diagnostics.Bool(err == nil), ExitCode: diagnostics.Int(0),
		DurationMS: diagnostics.Int64(time.Since(e.startedAt).Milliseconds()),
		Error:      errorString(err),
	})
	return err
}

func (e *commandExecution) run(env []string) process.Result {
	ctx := e.config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return process.DefaultRunner{}.Run(ctx, process.TaskSpec{
		Command: e.request.Command, Args: e.request.Args, Dir: e.request.Dir, Env: env,
		Streams: process.Streams{Stdin: e.streams.Stdin, Stdout: e.stdout, Stderr: e.stderr},
	})
}

func (e *commandExecution) finish(result *process.Result) error {
	// A later masker (stderr merged into stdout) may feed an earlier one, so
	// flush in reverse order before closing paced output or reporting completion.
	flushStreamMaskers(e.maskers)
	emitProcessEndDiagnostics(e.diagnostic, e.diagnosticID, e.startedAt, result)
	if e.config.MetricsCallback != nil && result.Metrics != nil {
		e.config.MetricsCallback(result.Metrics)
	}
	if err := closePacedWriters(e.pacedClosers); err != nil && result.Err == nil {
		return err
	}
	if result.Err == nil {
		return nil
	}
	if result.Canceled {
		return result.Err
	}
	if result.ExitCode >= 0 {
		log.Debug("Command exited with non-zero code", "code", result.ExitCode)
		return errUtils.ExitCodeError{Code: result.ExitCode}
	}
	return result.Err
}
