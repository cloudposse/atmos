package tests

// MockTerminalDetector provides a mock implementation for testing terminal behavior.
type MockTerminalDetector struct {
	IsTTY     bool
	Width     int
	Height    int
	SizeError error
}

// IsTTYSupportForStdout returns the mocked TTY status for stdout.
func (m *MockTerminalDetector) IsTTYSupportForStdout() bool {
	return m.IsTTY
}

// IsTTYSupportForStderr returns the mocked TTY status for stderr.
func (m *MockTerminalDetector) IsTTYSupportForStderr() bool {
	return m.IsTTY
}

// GetStdoutSize returns the mocked terminal size for stdout.
func (m *MockTerminalDetector) GetStdoutSize() (int, int, error) {
	return m.Width, m.Height, m.SizeError
}

// GetStderrSize returns the mocked terminal size for stderr.
func (m *MockTerminalDetector) GetStderrSize() (int, int, error) {
	return m.Width, m.Height, m.SizeError
}

// IsTerminal returns the mocked TTY status for any file descriptor.
func (m *MockTerminalDetector) IsTerminal(fd int) bool {
	return m.IsTTY
}

// GetSize returns the mocked terminal size for any file descriptor.
func (m *MockTerminalDetector) GetSize(fd int) (int, int, error) {
	return m.Width, m.Height, m.SizeError
}
