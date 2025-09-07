package tui

import (
	"io"
)

// Message types for Bubble Tea.
type (
	// testCompleteMsg signals that all tests have completed.
	testCompleteMsg struct {
		exitCode int
	}
)

// testFailMsg signals that a test has failed.
type testFailMsg struct {
	test string
	pkg  string
}

// Additional message types.
type (
	// tickMsg is sent periodically to update the UI.
	tickMsg struct{}
)

// subprocessReadyMsg signals that the subprocess is ready to start reading output.
type subprocessReadyMsg struct {
	proc io.ReadCloser
}

// streamOutputMsg contains a line of output from the test process.
type streamOutputMsg struct {
	line string
}
