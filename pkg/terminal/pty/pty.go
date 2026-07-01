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
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
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
	return runWithIO(ctx, &ioRunConfig{
		cmd:                      cmd,
		ptmx:                     ptmx,
		stdin:                    stdin,
		stdout:                   outputWriter,
		emulateTerminalResponses: opts.DisableStdinForward,
	})
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
func createOutputWriter(opts *Options) io.Writer {
	if opts.EnableMasking && opts.Masker != nil && opts.Masker.Enabled() {
		return &recordingWriter{
			underlying: opts.Stdout,
			masker:     opts.Masker,
		}
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
	return &terminalResponseReader{src: ptmx, responder: ptmx}
}

type terminalResponseReader struct {
	src       io.Reader
	responder io.Writer
	tail      string
}

const terminalResponseTailLen = 64

func (r *terminalResponseReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		previousTailLen := len(r.tail)
		output := r.tail + string(p[:n])
		r.respond(output, previousTailLen)
		r.tail = lastN(output, terminalResponseTailLen)
	}
	return n, err
}

func (r *terminalResponseReader) respond(output string, previousTailLen int) {
	if containsNewSequence(output, "\x1b]10;?\x1b\\", previousTailLen) {
		_, _ = r.responder.Write([]byte("\x1b]10;rgb:ffff/ffff/ffff\x1b\\"))
	}
	if containsNewSequence(output, "\x1b]11;?\x1b\\", previousTailLen) {
		_, _ = r.responder.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
	}
	if containsNewSequence(output, "\x1b[6n", previousTailLen) {
		_, _ = r.responder.Write([]byte("\x1b[1;1R"))
	}
}

func containsNewSequence(output, sequence string, previousTailLen int) bool {
	for index := strings.Index(output, sequence); index >= 0; {
		if index+len(sequence) > previousTailLen {
			return true
		}
		nextOffset := index + 1
		next := strings.Index(output[nextOffset:], sequence)
		if next < 0 {
			return false
		}
		index = nextOffset + next
	}
	return false
}

func lastN(input string, n int) string {
	if len(input) <= n {
		return input
	}
	return input[len(input)-n:]
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
type recordingWriter struct {
	underlying io.Writer
	masker     iolib.Masker
}

// Write implements io.Writer by masking data before writing to underlying writer.
func (w *recordingWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "pty.recordingWriter.Write")()

	output := string(p)
	if w.masker != nil && w.masker.Enabled() {
		output = w.masker.Mask(output)
	}

	_, err = w.underlying.Write([]byte(output))
	if err != nil {
		return 0, err
	}

	iolib.RecordMaskedOutput(iolib.DataStream, output)

	// Return original byte count (not masked length) to maintain io.Writer contract.
	return len(p), nil
}
