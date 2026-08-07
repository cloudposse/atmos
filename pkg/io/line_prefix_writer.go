package io

import (
	"bytes"
	stdio "io"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	carriageReturnByte = '\r'
	lineFeedByte       = '\n'
)

var (
	crlfBytes = []byte{carriageReturnByte, lineFeedByte}
	crBytes   = []byte{carriageReturnByte}
	lfBytes   = []byte{lineFeedByte}
)

// LinePrefixWriter prefixes complete lines and serializes writes through a
// shared output lock. Partial lines are buffered until Flush or a line ending.
type LinePrefixWriter struct {
	mu      sync.Mutex
	writeMu *sync.Mutex
	prefix  string
	w       stdio.Writer
	buffer  []byte
	// pending holds the already-prefixed, already-normalized suffix of a line
	// that a prior write to w left unwritten (a short write or a write error
	// after n > 0 bytes). It is retried byte-for-byte before any new line, so
	// bytes w already accepted are never re-sent and the line's prefix is
	// never re-applied.
	pending []byte
}

// NewLinePrefixWriter creates a writer that prefixes every rendered line with
// "[prefix] ". A shared writeMu serializes writes across writers targeting the
// same terminal.
func NewLinePrefixWriter(prefix string, w stdio.Writer, writeMu *sync.Mutex) *LinePrefixWriter {
	if prefix != "" {
		prefix = "[" + prefix + "] "
	}
	return NewLinePrefixWriterRaw(prefix, w, writeMu)
}

// NewLinePrefixWriterRaw is like NewLinePrefixWriter but uses the prefix
// verbatim (no surrounding brackets or spacing), letting callers supply a
// pre-styled or colored label. A shared writeMu serializes writes across writers
// targeting the same terminal.
func NewLinePrefixWriterRaw(prefix string, w stdio.Writer, writeMu *sync.Mutex) *LinePrefixWriter {
	defer perf.Track(nil, "io.NewLinePrefixWriterRaw")()

	if writeMu == nil {
		writeMu = &sync.Mutex{}
	}
	return &LinePrefixWriter{
		writeMu: writeMu,
		prefix:  prefix,
		w:       w,
	}
}

func (w *LinePrefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	w.buffer = append(w.buffer, p...)

	// Hold writeMu for the whole flush so that every line produced by this
	// single Write call reaches the shared writer as one contiguous block.
	// Locking per-line let a concurrent node's writer interleave a line in
	// between two lines emitted from the same Write call.
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if err := w.flushCompleteLinesLocked(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush writes any trailing partial line, plus any complete lines still buffered.
func (w *LinePrefixWriter) Flush() error {
	defer perf.Track(nil, "io.LinePrefixWriter.Flush")()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) == 0 && len(w.pending) == 0 {
		return nil
	}

	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	if err := w.flushCompleteLinesLocked(); err != nil {
		return err
	}
	if len(w.buffer) == 0 {
		return nil
	}
	line := append([]byte(nil), w.buffer...)
	w.buffer = w.buffer[:0]
	return w.writeLineLocked(line)
}

// flushCompleteLinesLocked writes any pending suffix left over from a prior
// short or failed write, then any buffered complete lines, while w.mu and
// w.writeMu are held. It leaves any trailing partial line buffered for a
// later Write or Flush.
func (w *LinePrefixWriter) flushCompleteLinesLocked() error {
	if len(w.pending) > 0 {
		if err := w.writePendingLocked(); err != nil {
			return err
		}
	}
	for {
		idx := lineEndIndex(w.buffer)
		if idx < 0 {
			return nil
		}
		end := idx + 1
		if w.buffer[idx] == carriageReturnByte && end < len(w.buffer) && w.buffer[end] == lineFeedByte {
			end++
		}
		line := append([]byte(nil), w.buffer[:end]...)
		w.buffer = w.buffer[end:]
		if err := w.writeLineLocked(line); err != nil {
			return err
		}
	}
}

// writeLineLocked writes one already-delimited raw line with the configured
// prefix. If the underlying write is short or fails after n > 0 bytes, the
// unwritten encoded suffix is kept in w.pending (not w.buffer) so a later
// call retries exactly those bytes, without re-applying the prefix or
// resending bytes w already accepted. Callers must hold w.writeMu.
func (w *LinePrefixWriter) writeLineLocked(line []byte) error {
	if w.w == nil {
		return nil
	}

	normalized := bytes.ReplaceAll(line, crlfBytes, lfBytes)
	normalized = bytes.ReplaceAll(normalized, crBytes, lfBytes)

	if w.prefix == "" {
		w.pending = normalized
	} else {
		w.pending = append([]byte(w.prefix), normalized...)
	}
	return w.writePendingLocked()
}

// writePendingLocked writes w.pending to the underlying writer, retaining
// only the unwritten suffix if the write is short or fails. A nil-error
// short write (n < len(w.pending) with err == nil) is treated as
// stdio.ErrShortWrite so callers still see and retry it. The caller must
// already hold w.writeMu.
func (w *LinePrefixWriter) writePendingLocked() error {
	n, err := w.w.Write(w.pending)
	if err == nil && n < len(w.pending) {
		err = stdio.ErrShortWrite
	}
	if err != nil {
		w.pending = append([]byte(nil), w.pending[n:]...)
		return err
	}
	w.pending = nil
	return nil
}

// lineEndIndex returns the first complete line-ending byte position or -1 when absent.
func lineEndIndex(p []byte) int {
	for i, c := range p {
		if c == lineFeedByte || (c == carriageReturnByte && i+1 < len(p)) {
			return i
		}
	}
	return -1
}
