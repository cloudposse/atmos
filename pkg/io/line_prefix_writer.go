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

// Flush writes any trailing partial line, plus any complete lines still buffered.
func (w *LinePrefixWriter) Flush() error {
	defer perf.Track(nil, "io.LinePrefixWriter.Flush")()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.buffer) == 0 {
		return nil
	}
	lines, rest := splitBufferedLines(w.buffer)
	if len(rest) > 0 {
		lines = append(lines, rest)
		rest = nil
	}
	return w.writeLinesLocked(lines, rest)
}

// flushCompleteLinesLocked writes buffered complete lines while w.mu is held,
// leaving any trailing partial line buffered for a later Write or Flush.
func (w *LinePrefixWriter) flushCompleteLinesLocked() error {
	lines, rest := splitBufferedLines(w.buffer)
	if len(lines) == 0 {
		return nil
	}
	return w.writeLinesLocked(lines, rest)
}

// splitBufferedLines splits buf into its complete, delimited lines and any
// trailing partial content, without mutating buf.
func splitBufferedLines(buf []byte) (lines [][]byte, rest []byte) {
	rest = buf
	for {
		idx := lineEndIndex(rest)
		if idx < 0 {
			return lines, rest
		}
		end := idx + 1
		if rest[idx] == carriageReturnByte && end < len(rest) && rest[end] == lineFeedByte {
			end++
		}
		lines = append(lines, append([]byte(nil), rest[:end]...))
		rest = rest[end:]
	}
}

// writeLinesLocked writes lines, in order, under a single writeMu acquisition
// so the whole batch lands on the shared underlying writer as one contiguous
// block instead of being interleaved line-by-line with a concurrent writer's
// own batch -- e.g. a \r-terminated segment held back by a prior Write plus
// the line that completes it in the next Write. On error, the line that
// failed and everything after it, plus rest, are restored to w.buffer so a
// later Write or Flush can retry them; nothing already written is repeated.
// The caller must already hold w.mu.
func (w *LinePrefixWriter) writeLinesLocked(lines [][]byte, rest []byte) error {
	if w.w == nil {
		w.buffer = rest
		return nil
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	for i, line := range lines {
		normalized := bytes.ReplaceAll(line, crlfBytes, lfBytes)
		normalized = bytes.ReplaceAll(normalized, crBytes, lfBytes)

		var err error
		if w.prefix == "" {
			_, err = w.w.Write(normalized)
		} else {
			_, err = stdio.WriteString(w.w, w.prefix+string(normalized))
		}
		if err != nil {
			w.buffer = append(joinLines(lines[i:]), rest...)
			return err
		}
	}
	w.buffer = rest
	return nil
}

// joinLines concatenates lines back into a single buffer, for restoring
// unwritten output after a partial-batch write failure.
func joinLines(lines [][]byte) []byte {
	var out []byte
	for _, line := range lines {
		out = append(out, line...)
	}
	return out
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
