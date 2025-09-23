package term

import (
	"os"

	"golang.org/x/term"
)

// TerminalDetector provides an interface for all terminal-related detection.
// This interface allows for mocking in tests while maintaining production behavior.
type TerminalDetector interface {
	// TTY detection
	IsTTYSupportForStdout() bool
	IsTTYSupportForStderr() bool

	// Terminal size detection
	GetStdoutSize() (width, height int, err error)
	GetStderrSize() (width, height int, err error)

	// File descriptor based methods for flexibility
	IsTerminal(fd int) bool
	GetSize(fd int) (width, height int, err error)
}

// Detector is the global terminal detector instance that can be swapped for testing.
// By default, it uses the real terminal implementation.
var Detector TerminalDetector = &DefaultTerminalDetector{}

// DefaultTerminalDetector uses the actual terminal for detection.
type DefaultTerminalDetector struct{}

// IsTTYSupportForStdout checks if stdout supports TTY.
func (d *DefaultTerminalDetector) IsTTYSupportForStdout() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsTTYSupportForStderr checks if stderr supports TTY.
func (d *DefaultTerminalDetector) IsTTYSupportForStderr() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// GetStdoutSize returns the size of the stdout terminal.
func (d *DefaultTerminalDetector) GetStdoutSize() (width, height int, err error) {
	return term.GetSize(int(os.Stdout.Fd()))
}

// GetStderrSize returns the size of the stderr terminal.
func (d *DefaultTerminalDetector) GetStderrSize() (width, height int, err error) {
	return term.GetSize(int(os.Stderr.Fd()))
}

// IsTerminal checks if the given file descriptor is a terminal.
func (d *DefaultTerminalDetector) IsTerminal(fd int) bool {
	return term.IsTerminal(fd)
}

// GetSize returns the size of the terminal for the given file descriptor.
func (d *DefaultTerminalDetector) GetSize(fd int) (width, height int, err error) {
	return term.GetSize(fd)
}
