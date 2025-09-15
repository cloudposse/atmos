package test

import (
	"runtime"
	"testing"
)

func TestSkipExample(t *testing.T) {
	t.Skip("SKIP: This test is skipped for demonstration purposes")
}

func TestSkipWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows: requires Unix-like file system")
	}
	// Test would proceed on non-Windows systems
}

func TestSkipReason(t *testing.T) {
	t.Skipf("Skipping test: example skip with formatted reason - test number %d", 42)
}

func TestAnotherSkip(t *testing.T) {
	t.Skip("SKIP: Feature not yet implemented")
}
