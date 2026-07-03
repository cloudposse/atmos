package terminal

import (
	"bytes"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// IsSpeedLimited reports whether terminal output should be paced.
// A speed of 0 means unlimited output, which preserves existing behavior.
func IsSpeedLimited(speed float64) bool {
	return speed > 0 && !math.IsNaN(speed) && !math.IsInf(speed, 0)
}

// IsTTYWriter reports whether w is an interactive terminal file.
func IsTTYWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	return term.IsTerminal(int(f.Fd())) //nolint:gosec // File descriptors are small OS-provided values accepted by x/term.
}

// PacingWriter reveals complete lines at a configured lines-per-second rate.
// Partial trailing output is buffered until another write completes the line or
// Close is called.
type PacingWriter struct {
	writer io.Writer
	delay  time.Duration
	sleep  func(time.Duration)

	mu     sync.Mutex
	buffer bytes.Buffer
}

// NewPacingWriter creates a writer that emits complete lines at speed lines per second.
func NewPacingWriter(writer io.Writer, speed float64) *PacingWriter {
	if !IsSpeedLimited(speed) {
		speed = 0
	}

	var delay time.Duration
	if speed > 0 {
		delay = time.Duration(float64(time.Second) / speed)
	}

	return &PacingWriter{
		writer: writer,
		delay:  delay,
		sleep:  time.Sleep,
	}
}

func (w *PacingWriter) Write(p []byte) (int, error) {
	if w == nil || w.writer == nil || len(p) == 0 {
		return len(p), nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.buffer.Write(p); err != nil {
		return 0, err
	}

	if err := w.flushCompleteLinesLocked(); err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close flushes any buffered partial trailing line immediately.
func (w *PacingWriter) Close() error {
	if w == nil || w.writer == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.buffer.Len() == 0 {
		return nil
	}

	_, err := w.writer.Write(w.buffer.Bytes())
	w.buffer.Reset()
	return err
}

func (w *PacingWriter) flushCompleteLinesLocked() error {
	for {
		pending := w.buffer.Bytes()
		idx := bytes.IndexByte(pending, '\n')
		if idx < 0 {
			return nil
		}

		line := make([]byte, idx+1)
		copy(line, pending[:idx+1])
		w.buffer.Next(idx + 1)

		if _, err := w.writer.Write(line); err != nil {
			return err
		}
		if w.delay > 0 {
			w.sleep(w.delay)
		}
	}
}
