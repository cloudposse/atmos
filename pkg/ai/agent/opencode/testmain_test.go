package opencode

import (
	"os"
	"testing"
)

// Fake-binary gate env vars. When set, the test binary impersonates the `opencode`
// CLI so SendMessage can be exercised cross-platform without a real binary or a
// platform-specific shell (see CLAUDE.md "Subprocess helpers in tests").
const (
	fakeStdoutEnv = "_ATMOS_OPENCODE_FAKE_STDOUT"
	fakeFailEnv   = "_ATMOS_OPENCODE_FAKE_FAIL"
)

// TestMain lets the test binary act as a fake `opencode` when a gate env var is set:
// it writes the canned stdout and/or exits non-zero, then returns before running tests.
func TestMain(m *testing.M) {
	if out := os.Getenv(fakeStdoutEnv); out != "" {
		_, _ = os.Stdout.WriteString(out)
		if os.Getenv(fakeFailEnv) == "1" {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv(fakeFailEnv) == "1" {
		_, _ = os.Stderr.WriteString("opencode: simulated failure")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
