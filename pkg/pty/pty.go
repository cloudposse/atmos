package pty

//go:generate go run go.uber.org/mock/mockgen@latest -source=pty.go -destination=mock_pty_test.go -package=pty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"

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

	// Apply default options.
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

	// Start the command with a PTY.
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	// Handle terminal resize signals.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				// Log error but don't fail - resize errors are non-fatal.
				continue
			}
		}
	}()
	ch <- syscall.SIGWINCH // Initial resize.
	defer func() { signal.Stop(ch); close(ch) }()

	// Set terminal to raw mode for proper PTY interaction (only if stdin is a TTY).
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		defer func() {
			if oldState != nil {
				_ = term.Restore(int(os.Stdin.Fd()), oldState)
			}
		}()
	}

	// Create output writer with optional masking.
	outputWriter := opts.Stdout
	if opts.EnableMasking && opts.Masker != nil && opts.Masker.Enabled() {
		outputWriter = &maskedWriter{
			underlying: opts.Stdout,
			masker:     opts.Masker,
		}
	}

	// Set up bidirectional IO between terminal and PTY.
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// Copy input from user terminal to PTY.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(ptmx, opts.Stdin)
		// Ignore EIO errors - these occur normally when PTY slave closes.
		// This happens when the shell exits (e.g., user types 'exit' or 'logout').
		// See: https://github.com/creack/pty/issues/21
		if err != nil && !isPtyEIO(err) {
			errChan <- fmt.Errorf("input copy failed: %w", err)
		}
	}()

	// Copy output from PTY to terminal (with optional masking).
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := io.Copy(outputWriter, ptmx)
		// Ignore EIO errors - these occur normally when PTY slave closes.
		// See: https://github.com/creack/pty/issues/21
		if err != nil && !isPtyEIO(err) {
			errChan <- fmt.Errorf("output copy failed: %w", err)
		}
	}()

	// Wait for command to complete or context cancellation.
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - kill the process.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		wg.Wait()
		return ctx.Err()
	case err := <-cmdDone:
		// Command completed - wait for IO to finish.
		wg.Wait()
		close(errChan)

		// Check for IO errors.
		for ioErr := range errChan {
			if ioErr != nil {
				return ioErr
			}
		}

		return err
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

// isPtyEIO checks if an error is the expected EIO error from reading a closed PTY.
//
// The Linux kernel returns EIO when attempting to read from a master pseudo-terminal
// which no longer has an open slave. This is normal behavior and not an error condition.
//
// See: https://github.com/creack/pty/issues/21
func isPtyEIO(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, syscall.EIO)
	}
	return false
}

// maskedWriter wraps an io.Writer to apply masking to all written data.
type maskedWriter struct {
	underlying io.Writer
	masker     iolib.Masker
}

// Write implements io.Writer by masking data before writing to underlying writer.
func (m *maskedWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "pty.maskedWriter.Write")()

	// Convert bytes to string, apply masking, write back.
	original := string(p)
	masked := m.masker.Mask(original)

	// Write masked data to underlying writer.
	written, err := m.underlying.Write([]byte(masked))
	if err != nil {
		return written, err
	}

	// Return original byte count (not masked length) to maintain io.Writer contract.
	return len(p), nil
}
