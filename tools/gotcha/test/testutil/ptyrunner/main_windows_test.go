//go:build windows
// +build windows

package main

import (
	"testing"
)

func TestPTYRunnerOnWindows(t *testing.T) {
	t.Skipf("ptyrunner tests skipped on Windows: PTYs (pseudo-terminals) are not supported on Windows. This is a Unix-only test utility for testing gotcha's TUI mode.")
}
