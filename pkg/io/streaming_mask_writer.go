package io

import (
	"errors"
	stdio "io"
	"os"
	"sync"

	xterm "github.com/charmbracelet/x/term"

	"github.com/cloudposse/atmos/pkg/perf"
)

// StreamingMaskWriter masks secrets in a stream whose writes may split a secret anywhere.
//
// Unlike MaskWriter, which masks every Write independently, it carries over the unfinished tail
// of a write that could still grow into a registered secret and masks it together with the next
// write. With only literals registered, output that cannot be the start of a secret is forwarded
// on the same Write call. With regex patterns registered, the entire unfinished line is held by
// default, so buffering grows with the line length until a newline, carriage return, or Flush.
// Bounded cross-line regexes also retain their maximum match width and any line crossing that
// boundary. Unbounded cross-line regexes are rejected when registered.
// Held bytes are released only by a later Write that rules them out, or by Flush; nothing is ever
// flushed on a timer.
//
// The writer is safe for concurrent use. Call Flush once the producer is done.
type StreamingMaskWriter struct {
	mu           sync.Mutex
	underlying   stdio.Writer
	masker       Masker
	lineBoundary bool
	pending      []byte
}

// StreamingMaskOption configures a StreamingMaskWriter.
type StreamingMaskOption func(*StreamingMaskWriter)

// WithoutLineHold disables holding the unfinished last line while regex patterns are registered.
// Registered literals stay protected from split writes. Use it for interactive streams where a
// prompt without a trailing newline must be shown immediately.
func WithoutLineHold() StreamingMaskOption {
	defer perf.Track(nil, "io.WithoutLineHold")()

	return func(w *StreamingMaskWriter) {
		w.lineBoundary = false
	}
}

// WithStreamMasker overrides the masker the writer uses instead of the global one. Use it when the caller
// already owns a Masker (for example the PTY bridge, which receives it through its options).
func WithStreamMasker(masker Masker) StreamingMaskOption {
	defer perf.Track(nil, "io.WithStreamMasker")()

	return withStreamingMasker(masker)
}

// withStreamingMasker overrides the masker (used by tests).
func withStreamingMasker(masker Masker) StreamingMaskOption {
	return func(w *StreamingMaskWriter) {
		w.masker = masker
	}
}

// MaskOptionsForStdin returns the streaming-mask options for a process whose input is stdin.
// When stdin is an interactive terminal, prompts without a trailing newline must appear promptly,
// so the unfinished line is not held for regex patterns (registered literals stay protected).
// Non-interactive input keeps the default line-boundary hold.
func MaskOptionsForStdin(stdin stdio.Reader) []StreamingMaskOption {
	defer perf.Track(nil, "io.MaskOptionsForStdin")()

	if f, ok := stdin.(*os.File); ok && f != nil && xterm.IsTerminal(f.Fd()) {
		return []StreamingMaskOption{WithoutLineHold()}
	}
	return nil
}

// MaskedStreams pairs a stdout and a stderr streaming masker for one process run. Both are
// flushed together once the process has exited so no held tail is lost.
type MaskedStreams struct {
	// Stdout and Stderr are the masked writers to hand to the process. Either is nil when no
	// target was supplied for it.
	Stdout stdio.Writer
	Stderr stdio.Writer

	maskers []*StreamingMaskWriter
}

// NewMaskedStreams wraps stdout and stderr (either may be nil) with split-safe masking.
// The stdin argument selects interactive behavior as described by MaskOptionsForStdin; pass
// os.Stdin for processes that inherit the terminal.
func NewMaskedStreams(stdin stdio.Reader, stdout, stderr stdio.Writer) *MaskedStreams {
	defer perf.Track(nil, "io.NewMaskedStreams")()

	opts := MaskOptionsForStdin(stdin)
	ms := &MaskedStreams{}
	if stdout != nil {
		sw := NewStreamingMaskWriter(stdout, opts...)
		ms.Stdout = sw
		ms.maskers = append(ms.maskers, sw)
	}
	if stderr != nil {
		sw := NewStreamingMaskWriter(stderr, opts...)
		ms.Stderr = sw
		ms.maskers = append(ms.maskers, sw)
	}
	return ms
}

// Flush releases the tail held by each masker. Call it after the process has exited.
func (ms *MaskedStreams) Flush() error {
	defer perf.Track(nil, "io.MaskedStreams.Flush")()

	var errs []error
	for i := len(ms.maskers) - 1; i >= 0; i-- {
		errs = append(errs, ms.maskers[i].Flush())
	}
	return errors.Join(errs...)
}

// NewStreamingMaskWriter wraps w with split-safe masking backed by the global masker.
// When the global I/O context cannot be initialized the writer passes data through unchanged,
// matching MaskWriter.
func NewStreamingMaskWriter(w stdio.Writer, opts ...StreamingMaskOption) *StreamingMaskWriter {
	defer perf.Track(nil, "io.NewStreamingMaskWriter")()

	sw := &StreamingMaskWriter{underlying: w, lineBoundary: true}
	if ctx := GetContext(); ctx != nil {
		sw.masker = ctx.Masker()
	}
	for _, opt := range opts {
		opt(sw)
	}
	return sw
}

// Write implements io.Writer. It reports len(p) once the bytes are accepted, even when a tail is
// held back for the next Write.
func (sw *StreamingMaskWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "io.StreamingMaskWriter.Write")()

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.masker == nil {
		return sw.underlying.Write(p)
	}

	input := string(p)
	if len(sw.pending) > 0 {
		input = string(sw.pending) + input
	}

	cut := len(input) - sw.masker.HoldbackLen(input, sw.lineBoundary)
	sw.pending = append(sw.pending[:0], input[cut:]...)
	if cut == 0 {
		return len(p), nil
	}

	if err := sw.emit(input[:cut]); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush masks and writes any held tail. Call it when the producer has finished.
func (sw *StreamingMaskWriter) Flush() error {
	defer perf.Track(nil, "io.StreamingMaskWriter.Flush")()

	sw.mu.Lock()
	defer sw.mu.Unlock()

	if len(sw.pending) == 0 {
		return nil
	}
	input := string(sw.pending)
	sw.pending = sw.pending[:0]
	return sw.emit(input)
}

// emit masks chunk and writes it to the underlying writer. Must be called with the lock held.
func (sw *StreamingMaskWriter) emit(chunk string) error {
	masked := sw.masker.Mask(chunk)
	written, err := sw.underlying.Write([]byte(masked))
	if written > 0 {
		// Mirror MaskWriter: output headed for the process streams is visible to the cast recorder.
		recorded := masked[:min(written, len(masked))]
		switch sw.underlying {
		case os.Stdout:
			recordOutput(DataStream, recorded)
		case os.Stderr:
			recordOutput(UIStream, recorded)
		}
	}
	if err != nil {
		return err
	}
	if written < len(masked) {
		return stdio.ErrShortWrite
	}
	return nil
}

// Finish flushes the maskers and returns runErr unchanged when it is set, so error identity (for
// example exit-code errors) survives. When runErr is nil it returns the flush error, if any.
func (ms *MaskedStreams) Finish(runErr error) error {
	defer perf.Track(nil, "io.MaskedStreams.Finish")()

	flushErr := ms.Flush()
	if runErr != nil {
		return runErr
	}
	return flushErr
}
