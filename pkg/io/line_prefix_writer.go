package io

import (
	"bytes"
	stdio "io"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// LinePrefixWriter prefixes complete lines and serializes writes through a
// shared output lock. Partial lines are buffered until Flush or a line ending.
type LinePrefixWriter struct {
	mu      sync.Mutex
	writeMu *sync.Mutex
	prefix  string
	w       stdio.Writer
	buffer  []byte
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
	if err := w.flushCompleteLinesLocked(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush writes any trailing partial line.
func (w *LinePrefixWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) == 0 {
		return nil
	}
	if err := w.flushCompleteLinesLocked(); err != nil {
		return err
	}
	if len(w.buffer) == 0 {
		return nil
	}
	line := append([]byte(nil), w.buffer...)
	if err := w.writeLine(line); err != nil {
		return err
	}
	w.buffer = w.buffer[:0]
	return nil
}

// flushCompleteLinesLocked writes buffered complete lines while w.mu is held.
func (w *LinePrefixWriter) flushCompleteLinesLocked() error {
	for {
		idx := lineEndIndex(w.buffer)
		if idx < 0 {
			return nil
		}
		end := idx + 1
		if w.buffer[idx] == '\r' && end < len(w.buffer) && w.buffer[end] == '\n' {
			end++
		}
		line := append([]byte(nil), w.buffer[:end]...)
		if err := w.writeLine(line); err != nil {
			return err
		}
		w.buffer = w.buffer[end:]
	}
}

// writeLine writes one already-delimited line with the configured prefix.
func (w *LinePrefixWriter) writeLine(line []byte) error {
	if w.w == nil {
		return nil
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	if w.prefix == "" {
		_, err := w.w.Write(line)
		return err
	}

	line = bytes.ReplaceAll(line, []byte("\r\n"), []byte("\n"))
	line = bytes.ReplaceAll(line, []byte("\r"), []byte("\n"))
	_, err := stdio.WriteString(w.w, w.prefix+string(line))
	return err
}

// lineEndIndex returns the first complete line-ending byte position or -1 when absent.
func lineEndIndex(p []byte) int {
	for i, c := range p {
		if c == '\n' || (c == '\r' && i+1 < len(p)) {
			return i
		}
	}
	return -1
}
