package term

import (
	"io"
	"os"

	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/term"
)

// TerminalWriter wraps an io.Writer and provides automatic line wrapping based on terminal width
// It ensures that output text is formatted to fit within the terminal's dimensions.
type TerminalWriter struct {
	width  uint
	writer io.Writer
}

const (
	maxWidth    = 120
	mediumWidth = 100
	minWidth    = 80
)

// NewResponsiveWriter creates a terminal-aware writer that automatically wraps text
// based on the terminal width. If the provided writer is not a terminal or if width
// detection fails, it will return the original writer unchanged.
func NewResponsiveWriter(w io.Writer) io.Writer {
	file, ok := w.(*os.File)
	if !ok {
		return w
	}

	if !IsTTYSupportForStdout() {
		return w
	}

	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return w
	}

	// Use optimal width based on terminal size
	var limit uint

	switch {
	case width >= maxWidth:
		limit = maxWidth
	case width >= mediumWidth:
		limit = mediumWidth
	case width >= minWidth:
		limit = minWidth
	default:
		limit = uint(width)
	}

	return &TerminalWriter{
		width:  limit,
		writer: w,
	}
}

func (w *TerminalWriter) Write(p []byte) (int, error) {
	if w.width == 0 {
		return w.writer.Write(p)
	}

	// Preserving the original length for correct return value
	originalLen := len(p)
	wrapped := wordwrap.WrapString(string(p), w.width)

	n, err := w.writer.Write([]byte(wrapped))
	if err != nil {
		return n, err
	}
	// return the original length as per io.Writer contract
	return originalLen, nil
}

func (w *TerminalWriter) GetWidth() uint {
	return w.width
}

// CheckTTYSupportStdout checks if stdout supports TTY for displaying the progress UI.
func IsTTYSupportForStdout() bool {
	fd := int(os.Stdout.Fd())
	isTerminal := term.IsTerminal(fd)

	return isTerminal
}

// CheckTTYSupportStderr checks if stderr supports TTY for displaying the progress UI.
func IsTTYSupportForStderr() bool {
	fd := int(os.Stderr.Fd())
	isTerminal := term.IsTerminal(fd)

	return isTerminal
}
