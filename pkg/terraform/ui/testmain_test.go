package ui

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain lets the test binary itself stand in for a long-running subprocess (cross-platform,
// per CLAUDE.md - avoids relying on platform-specific binaries like `sleep`). If
// _ATMOS_TEST_SLEEP_SECONDS is set, the binary sleeps for that many seconds and exits 0 instead
// of running the test suite; used by TestKillIfCancelled_KillsRealProcess.
func TestMain(m *testing.M) {
	if secs := os.Getenv("_ATMOS_TEST_SLEEP_SECONDS"); secs != "" {
		if n, err := strconv.Atoi(secs); err == nil {
			time.Sleep(time.Duration(n) * time.Second)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
