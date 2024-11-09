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

func NewResponsiveWriter(w io.Writer) io.Writer {
	file, ok := w.(*os.File)
	if !ok {
		return w
	}

	if !term.IsTerminal(int(file.Fd())) {
		return w
	}

	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return w
	}

	// Use optimal width based on terminal size
	var limit uint
	switch {
	case width >= 120:
		limit = 120
	case width >= 100:
		limit = 100
	case width >= 80:
		limit = 80
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
	wrapped := wordwrap.WrapString(string(p), w.width)
	return w.writer.Write([]byte(wrapped))
}
