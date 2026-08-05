package io

import (
	stdio "io"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// streams implements the Streams interface.
// Note: rawOutput/rawError are not cached - RawOutput()/RawError() return os.Stdout/os.Stderr
// dynamically to support test output capture via os.Stdout redirection.
type streams struct {
	input  stdio.Reader
	output stdio.Writer // Masked
	error  stdio.Writer // Masked
	masker Masker
	config *Config
}

// newStreams creates a new Streams with automatic masking.
// Output and error streams use dynamic references to os.Stdout/os.Stderr
// to support test output capture via os.Stdout redirection.
func newStreams(masker Masker, config *Config) Streams {
	s := &streams{
		input:  os.Stdin,
		masker: masker,
		config: config,
	}

	// Wrap output and error with masking (if enabled).
	// Use dynamic writers that resolve os.Stdout/os.Stderr at write time,
	// allowing tests to capture output by redirecting these variables.
	if masker.Enabled() {
		s.output = &dynamicMaskedWriter{
			stream:    DataStream,
			masker:    masker,
			getWriter: func() stdio.Writer { return os.Stdout },
		}
		s.error = &dynamicMaskedWriter{
			stream:    UIStream,
			masker:    masker,
			getWriter: func() stdio.Writer { return os.Stderr },
		}
	} else {
		s.output = &dynamicWriter{stream: DataStream, getWriter: func() stdio.Writer { return os.Stdout }}
		s.error = &dynamicWriter{stream: UIStream, getWriter: func() stdio.Writer { return os.Stderr }}
	}

	return s
}

func (s *streams) Input() stdio.Reader {
	defer perf.Track(nil, "io.streams.Input")()

	return s.input
}

func (s *streams) Output() stdio.Writer {
	defer perf.Track(nil, "io.streams.Output")()

	return s.output
}

func (s *streams) Error() stdio.Writer {
	defer perf.Track(nil, "io.streams.Error")()

	return s.error
}

func (s *streams) RawOutput() stdio.Writer {
	defer perf.Track(nil, "io.streams.RawOutput")()

	// Return os.Stdout dynamically to support test output capture.
	return os.Stdout
}

func (s *streams) RawError() stdio.Writer {
	defer perf.Track(nil, "io.streams.RawError")()

	// Return os.Stderr dynamically to support test output capture.
	return os.Stderr
}

// dynamicWriter wraps a dynamic writer getter for unmasked output.
// This allows the underlying writer to be resolved at write time,
// supporting test output capture via os.Stdout redirection.
type dynamicWriter struct {
	stream    Stream
	getWriter func() stdio.Writer
}

// Write implements stdio.Writer by delegating to the dynamic writer.
func (dw *dynamicWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.dynamicWriter.Write")()

	written, err := dw.getWriter().Write(p)
	if written > 0 {
		recordOutput(dw.stream, string(p[:min(written, len(p))]))
	}
	if err != nil {
		return written, err
	}
	if written < len(p) {
		return written, stdio.ErrShortWrite
	}
	return written, nil
}

// invalidFd is the sentinel returned by maskedWriter.Fd() when the underlying
// writer doesn't expose a real file descriptor. It deliberately uses a value
// that can never be a valid fd (all bits set) rather than 0: 0 is stdin's real
// fd number on POSIX, so returning 0 here would make callers like
// term.IsTerminal(fd) incorrectly probe stdin's terminal-ness instead of
// correctly reporting "not a terminal" for this writer.
const invalidFd = ^uintptr(0)

// maskedWriter wraps a stdio.Writer and automatically masks sensitive data.
// Used by MaskWriter() to wrap arbitrary writers with masking.
//
// It also implements Fd()/Read()/Close() so that, when it wraps a real
// *os.File (e.g. os.Stdout), it transparently satisfies capability
// interfaces like term.File (github.com/charmbracelet/x/term) that TTY
// detection code type-asserts against. Without this, wrapping os.Stdout with
// MaskWriter makes it indistinguishable from a non-terminal writer to callers
// like Bubble Tea, which silently disables their own terminal-size detection,
// cursor hiding, and line-clearing behavior. See the Bubble Tea program setup
// in internal/exec/vendor_model.go for the motivating case.
type maskedWriter struct {
	underlying stdio.Writer
	masker     Masker
}

// Fd implements the fd-capability interface (e.g. term.File) used by TTY
// detection code. It delegates to the underlying writer's Fd() when
// available (e.g. *os.File), so wrapping a real terminal file remains
// transparent to that detection. Returns invalidFd when the underlying
// writer doesn't expose a real file descriptor (e.g. a bytes.Buffer, or a
// nil underlying writer), so syscalls against it harmlessly fail instead of
// aliasing an unrelated real fd (like stdin's fd 0).
func (mw *maskedWriter) Fd() uintptr {
	defer perf.Track(nil, "io.maskedWriter.Fd")()

	if f, ok := mw.underlying.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return invalidFd
}

// Read implements io.Reader so *maskedWriter can satisfy full read/write/close
// file-handle interfaces (e.g. term.File) required by some TTY-detection type
// assertions. It is fundamentally an output-only wrapper -- the only consumer
// that needs this (Bubble Tea) reads input via a separate tea.WithInput
// reader and never reads from the configured output writer. Delegate to the
// underlying reader if it happens to implement one (unlikely for
// stdout/stderr), otherwise report EOF rather than panicking.
func (mw *maskedWriter) Read(p []byte) (int, error) {
	defer perf.Track(nil, "io.maskedWriter.Read")()

	if r, ok := mw.underlying.(stdio.Reader); ok {
		return r.Read(p)
	}
	return 0, stdio.EOF
}

// Close implements io.Closer. It intentionally never closes the process-wide
// os.Stdout/os.Stderr handles, even though *os.File implements io.Closer --
// closing either would break all subsequent output for the rest of the
// process. Any other underlying io.Closer (e.g. a real log file) is
// delegated to; writers that aren't closers are a no-op.
func (mw *maskedWriter) Close() error {
	defer perf.Track(nil, "io.maskedWriter.Close")()

	switch mw.underlying {
	case os.Stdout, os.Stderr:
		return nil
	}
	if c, ok := mw.underlying.(stdio.Closer); ok {
		return c.Close()
	}
	return nil
}

// Write implements stdio.Writer with automatic masking.
func (mw *maskedWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.maskedWriter.Write")()

	input := string(p)
	masked := mw.masker.Mask(input)
	maskedBytes := []byte(masked)

	written, err := mw.underlying.Write(maskedBytes)
	if written > 0 {
		recorded := string(maskedBytes[:min(written, len(maskedBytes))])
		switch mw.underlying {
		case os.Stdout:
			recordOutput(DataStream, recorded)
		case os.Stderr:
			recordOutput(UIStream, recorded)
		}
	}
	if err != nil {
		return 0, err
	}

	// Check for partial write.
	if written < len(maskedBytes) {
		return 0, stdio.ErrShortWrite
	}

	// Return original length to maintain write semantics.
	return len(p), nil
}

// dynamicMaskedWriter wraps a dynamic writer getter with automatic masking.
// This allows the underlying writer to be resolved at write time,
// supporting test output capture via os.Stdout redirection.
type dynamicMaskedWriter struct {
	stream    Stream
	masker    Masker
	getWriter func() stdio.Writer
}

// Write implements stdio.Writer with automatic masking and dynamic writer resolution.
func (dmw *dynamicMaskedWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "io.dynamicMaskedWriter.Write")()

	input := string(p)
	masked := dmw.masker.Mask(input)
	maskedBytes := []byte(masked)

	written, err := dmw.getWriter().Write(maskedBytes)
	if written > 0 {
		recordOutput(dmw.stream, string(maskedBytes[:min(written, len(maskedBytes))]))
	}
	if err != nil {
		return 0, err
	}

	// Check for partial write.
	if written < len(maskedBytes) {
		return 0, stdio.ErrShortWrite
	}

	// Return original length to maintain write semantics.
	return len(p), nil
}
