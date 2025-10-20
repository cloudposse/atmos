package term

import (
	"io"
	"os"

	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/term"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=term_writer.go -destination=mock_term_writer.go -package=term

// TTYDetector provides an interface for detecting TTY support.
// This allows for testing by injecting mock implementations.
type TTYDetector interface {
	// IsTTYForStdout checks if stdout supports TTY.
	IsTTYForStdout() bool
	// IsTTYForStderr checks if stderr supports TTY.
	IsTTYForStderr() bool
	// IsTTYForStdin checks if stdin supports TTY (interactive input).
	IsTTYForStdin() bool
}

// DefaultTTYDetector implements TTYDetector using the actual terminal checks.
type DefaultTTYDetector struct{}

// IsTTYForStdout checks if stdout supports TTY.
func (d *DefaultTTYDetector) IsTTYForStdout() bool {
	fd := int(os.Stdout.Fd())
	return term.IsTerminal(fd)
}

// IsTTYForStderr checks if stderr supports TTY.
func (d *DefaultTTYDetector) IsTTYForStderr() bool {
	fd := int(os.Stderr.Fd())
	return term.IsTerminal(fd)
}

// IsTTYForStdin checks if stdin supports TTY (interactive input).
func (d *DefaultTTYDetector) IsTTYForStdin() bool {
	fd := int(os.Stdin.Fd())
	return term.IsTerminal(fd)
}

// defaultDetector is the global instance used by package-level functions.
var defaultDetector TTYDetector = &DefaultTTYDetector{}

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
		// TODO: Why did we have this limit. My terminal does not work as per expectations for long sentences in markdown
		// limit = maxWidth
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

// IsTTYSupportForStdout checks if stdout supports TTY for displaying the progress UI.
// This is a convenience function that uses the default TTY detector.
func IsTTYSupportForStdout() bool {
	return defaultDetector.IsTTYForStdout()
}

// IsTTYSupportForStderr checks if stderr supports TTY for displaying the progress UI.
// This is a convenience function that uses the default TTY detector.
func IsTTYSupportForStderr() bool {
	return defaultDetector.IsTTYForStderr()
}

// IsTTYSupportForStdin checks if stdin supports TTY for accepting interactive input.
// This is a convenience function that uses the default TTY detector.
func IsTTYSupportForStdin() bool {
	return defaultDetector.IsTTYForStdin()
}
