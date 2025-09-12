package tui

import (
	"io"
)

// Message types for Bubble Tea.
type (
	// TestCompleteMsg signals that all tests have completed.
	TestCompleteMsg struct {
		ExitCode int
	}
)

// testFailMsg signals that a test has failed.
type testFailMsg struct {
	test string
	pkg  string
}

// Additional message types.
type (
	// TickMsg is sent periodically to update the UI.
	tickMsg struct{}
)

// subprocessReadyMsg signals that the subprocess is ready to start reading output.
type subprocessReadyMsg struct {
	proc io.ReadCloser
}

// StreamOutputMsg contains a line of output from the test process.
type StreamOutputMsg struct {
	Line string
}
