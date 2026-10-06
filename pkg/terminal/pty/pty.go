package pty

//go:generate go run go.uber.org/mock/mockgen@latest -source=pty.go -destination=mock_pty_test.go -package=pty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal/query"
)

// Options represents configuration for PTY execution.
type Options struct {
	// Masker is the masking implementation from pkg/io.
	Masker iolib.Masker

	// EnableMasking enables output masking through the PTY proxy.
	EnableMasking bool

	// DisableStdinForward skips forwarding Stdin to the PTY and skips putting
	// the host terminal into raw mode (like docker -t without -i: the child
	// gets a TTY but receives no input from the host).
	DisableStdinForward bool

	// Stdin provides input to the PTY. If nil, defaults to os.Stdin.
	Stdin io.Reader

	// Stdout receives output from the PTY. If nil, defaults to os.Stdout.
	Stdout io.Writer

	// Stderr receives error output from the PTY. Note: PTY merges stderr with stdout.
	// This is preserved for API consistency but data will not flow here in PTY mode.
	Stderr io.Writer
}

// ExecWithPTY executes a command in a pseudo-terminal with optional output masking.
//
// This function provides TTY emulation while allowing masking of sensitive data in output.
// It integrates with Atmos's existing pkg/io masking infrastructure.
//
// Platform Support:
//   - macOS: Fully supported
//   - Linux: Fully supported
//   - Windows: Not supported (use regular exec.Cmd.Run instead)
//
// Limitations:
//   - PTY merges stderr and stdout into single stream
//   - EIO errors may occur when reading from closed PTY (this is normal)
//   - Terminal size must be synchronized with host terminal
//
// Example:
//
//	ctx := context.Background()
//	cmd := exec.Command("docker", "exec", "-it", containerID, "bash")
//	opts := &Options{
//	    Masker:        ioCtx.Masker(),
//	    EnableMasking: true,
//	}
//	err := ExecWithPTY(ctx, cmd, opts)
func ExecWithPTY(ctx context.Context, cmd *exec.Cmd, opts *Options) error {
	defer perf.Track(nil, "pty.ExecWithPTY")()

	// Validate platform support.
	if !IsSupported() {
		return fmt.Errorf("%w: %s", errUtils.ErrPTYNotSupported, runtime.GOOS)
	}

	// Apply defaults and start PTY.
	opts = applyDefaults(opts)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Setup terminal environment. Raw mode is only needed when host input is
	// forwarded to the PTY (it routes control bytes like Ctrl-C to the child).
	cleanup, err := setupTerminal(ptmx, !opts.DisableStdinForward)
	if err != nil {
		return err
	}
	defer cleanup()

	// Create output writer with optional masking.
	outputWriter := createOutputWriter(opts)

	// Run command with bidirectional IO.
	stdin := opts.Stdin
	if opts.DisableStdinForward {
		stdin = nil
	}
	runErr := runWithIO(ctx, &ioRunConfig{
		cmd:                      cmd,
		ptmx:                     ptmx,
		stdin:                    stdin,
		stdout:                   outputWriter,
		emulateTerminalResponses: shouldEmulateTerminalResponses(opts),
	})

	// The output copier has finished, so release any tail the masker held back.
	if flushErr := outputWriter.Flush(); flushErr != nil && runErr == nil {
		return fmt.Errorf("failed to flush masked PTY output: %w", flushErr)
	}
	return runErr
}

// shouldEmulateTerminalResponses reports whether Atmos must answer terminal queries
// (OSC 10/11, CSI 6n) itself. A real host terminal reached through forwarded stdin
// answers them; without one (no forwarded input, or stdin that is not a terminal)
// children such as bubbletea/termenv programs would wait seconds for each reply.
func shouldEmulateTerminalResponses(opts *Options) bool {
	if opts.DisableStdinForward {
		return true
	}
	f, ok := opts.Stdin.(*os.File)
	if !ok {
		return true
	}
	return !term.IsTerminal(int(f.Fd()))
}

// applyDefaults applies default values to Options if not set.
func applyDefaults(opts *Options) *Options {
	if opts == nil {
		opts = &Options{}
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	return opts
}

// createOutputWriter creates an output writer with optional masking.
func createOutputWriter(opts *Options) *recordingWriter {
	if opts.EnableMasking && opts.Masker != nil && opts.Masker.Enabled() {
		w := &recordingWriter{
			underlying: opts.Stdout,
			masker:     opts.Masker,
		}
		if !opts.DisableStdinForward {
			// A forwarded interactive terminal must see prompts without a trailing newline promptly.
			w.streamOpts = iolib.MaskOptionsForStdin(opts.Stdin)
		}
		return w
	}
	return &recordingWriter{underlying: opts.Stdout}
}

type ioRunConfig struct {
	cmd                      *exec.Cmd
	ptmx                     *os.File
	stdin                    io.Reader
	stdout                   io.Writer
	emulateTerminalResponses bool
}

// runWithIO sets up bidirectional IO and waits for command completion.
// The stdin reader may be nil to skip input forwarding entirely.
func runWithIO(ctx context.Context, cfg *ioRunConfig) error {
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Copy input from user terminal to PTY. This goroutine is intentionally
	// NOT in the WaitGroup: io.Copy from a terminal stdin only returns on the
	// next read after the PTY closes, so joining it would block completion
	// until the user presses a key (the standard docker-CLI pattern is to let
	// it die with the process).
	if cfg.stdin != nil {
		go copyInput(errChan, cfg.ptmx, cfg.stdin)
	}

	// Copy output from PTY to terminal.
	wg.Add(1)
	go copyOutput(&wg, errChan, cfg.stdout, newOutputReader(cfg.ptmx, cfg.emulateTerminalResponses))

	// Wait for completion or cancellation.
	return waitForCompletion(ctx, cfg.cmd, cfg.ptmx, &wg, errChan)
}

// copyInput copies data from stdin to PTY, ignoring expected EIO errors.
// The send is non-blocking: input errors observed after completion has already
// drained the channel are not actionable.
func copyInput(errChan chan error, dst io.Writer, src io.Reader) {
	_, err := io.Copy(dst, src)
	if err != nil && !isPtyEIO(err) {
		select {
		case errChan <- fmt.Errorf("input copy failed: %w", err):
		default:
		}
	}
}

func newOutputReader(ptmx *os.File, emulateTerminalResponses bool) io.Reader {
	if !emulateTerminalResponses {
		return ptmx
	}
	return query.NewReader(ptmx, ptmx)
}

// outputDrainTimeout bounds how long completion waits for the output copier
// after the child exits. The copier normally ends almost immediately with EIO
// when the last PTY slave fd closes - but grandchildren that inherited the
// slave (e.g. aws ssm's session-manager-plugin, or backgrounded processes)
// can keep it open indefinitely, which would leave the host terminal in raw
// mode with no prompt until a stray keypress shook things loose.
const outputDrainTimeout = 1 * time.Second

// copyOutput copies data from PTY to stdout, ignoring expected errors:
// EIO from the closed PTY, and deadline/close errors from bounded draining.
func copyOutput(wg *sync.WaitGroup, errChan chan error, dst io.Writer, src io.Reader) {
	defer wg.Done()
	_, err := io.Copy(dst, src)
	if err != nil && !isPtyEIO(err) && !errors.Is(err, os.ErrDeadlineExceeded) && !errors.Is(err, os.ErrClosed) {
		errChan <- fmt.Errorf("output copy failed: %w", err)
	}
}

// waitOutputDrained waits for the output copier with a deadline. If the PTY
// slave is still held open past the deadline, the pending read is forced to
// return so the session can tear down and the host terminal can be restored.
func waitOutputDrained(ptmx *os.File, wg *sync.WaitGroup) {
	drained := make(chan struct{})
	go func() {
		wg.Wait()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(outputDrainTimeout):
		// Unblock the pending read: prefer a deadline (keeps ptmx usable for
		// the deferred Close), fall back to Close for non-pollable files.
		if err := ptmx.SetReadDeadline(time.Now()); err != nil {
			_ = ptmx.Close()
		}
		<-drained
	}
}

// waitForCompletion waits for command completion or context cancellation.
func waitForCompletion(ctx context.Context, cmd *exec.Cmd, ptmx *os.File, wg *sync.WaitGroup, errChan chan error) error {
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		waitOutputDrained(ptmx, wg)
		return ctx.Err()
	case err := <-cmdDone:
		waitOutputDrained(ptmx, wg)
		// Return first IO error if any, otherwise return command error.
		// Drain non-blockingly: the channel stays open because the detached
		// stdin copier may outlive command completion.
		for {
			select {
			case ioErr := <-errChan:
				if ioErr != nil {
					return ioErr
				}
			default:
				return err
			}
		}
	}
}

// IsSupported returns true if PTY operations are supported on this platform.
//
// Currently supported platforms:
//   - darwin (macOS)
//   - linux
//
// Not supported:
//   - windows (PTY operations require Unix-like system calls)
func IsSupported() bool {
	defer perf.Track(nil, "pty.IsSupported")()

	return runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

// recordingWriter wraps PTY output so terminal-attached steps are both masked
// and visible to the asciicast recorder. PTY output is terminal-like, so it is
// recorded as stdout even though the PTY merges stdout and stderr.
//
// PTY reads split output at arbitrary byte boundaries, so masking is stream-aware: a possible
// secret prefix at the end of one read is held back and masked together with the next read.
// Call Flush once the PTY output has been drained.
type recordingWriter struct {
	underlying io.Writer
	masker     iolib.Masker
	streamOpts []iolib.StreamingMaskOption

	once   sync.Once
	stream *iolib.StreamingMaskWriter
}

// Write implements io.Writer by masking data before writing to underlying writer.
func (w *recordingWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "pty.recordingWriter.Write")()

	if w.masker == nil {
		return recordedSink{w: w.underlying}.Write(p)
	}

	w.once.Do(func() {
		opts := append([]iolib.StreamingMaskOption{iolib.WithStreamMasker(w.masker)}, w.streamOpts...)
		w.stream = iolib.NewStreamingMaskWriter(recordedSink{w: w.underlying}, opts...)
	})
	return w.stream.Write(p)
}

// Flush writes any output the masker is still holding back. It is a no-op when masking is off.
func (w *recordingWriter) Flush() error {
	defer perf.Track(nil, "pty.recordingWriter.Flush")()

	if w.stream == nil {
		return nil
	}
	return w.stream.Flush()
}

// recordedSink writes already-masked PTY output to the underlying writer and the cast recorder.
type recordedSink struct {
	w io.Writer
}

// Write implements io.Writer.
func (s recordedSink) Write(p []byte) (int, error) {
	if _, err := s.w.Write(p); err != nil {
		return 0, err
	}

	iolib.RecordMaskedOutput(iolib.DataStream, string(p))

	// Return the byte count of p to keep the io.Writer contract.
	return len(p), nil
}
