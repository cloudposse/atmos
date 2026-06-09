package io

import (
	stdio "io"
	"strings"
	"sync"
)

// LinePrefixWriter prefixes complete lines and serializes writes through a
// shared output lock. Partial lines are buffered until Flush or a newline.
type LinePrefixWriter struct {
	mu      sync.Mutex
	writeMu *sync.Mutex
	prefix  string
	w       stdio.Writer
	buffer  []byte
}

// NewLinePrefixWriter creates a writer that prefixes every rendered line.
// writeMu may be shared across writers targeting the same terminal.
func NewLinePrefixWriter(prefix string, w stdio.Writer, writeMu *sync.Mutex) *LinePrefixWriter {
	if writeMu == nil {
		writeMu = &sync.Mutex{}
	}
	if prefix != "" {
		prefix = "[" + prefix + "] "
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
		idx := newlineIndex(w.buffer)
		if idx < 0 {
			return nil
		}
		line := append([]byte(nil), w.buffer[:idx+1]...)
		if err := w.writeLine(line); err != nil {
			return err
		}
		w.buffer = w.buffer[idx+1:]
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

	var b strings.Builder
	b.Grow(len(line) + len(w.prefix))
	for i, part := range strings.SplitAfter(string(line), "\r") {
		if part == "" {
			continue
		}
		if i == 0 || part != "\n" {
			b.WriteString(w.prefix)
		}
		b.WriteString(part)
	}
	_, err := stdio.WriteString(w.w, b.String())
	return err
}

// newlineIndex returns the first newline byte position or -1 when absent.
func newlineIndex(p []byte) int {
	for i, c := range p {
		if c == '\n' {
			return i
		}
	}
	return -1
}
