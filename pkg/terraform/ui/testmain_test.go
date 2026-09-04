package ui

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain lets the test binary itself stand in for a subprocess (cross-platform, per
// CLAUDE.md - avoids relying on platform-specific binaries like `sleep` or `false`).
//
//   - _ATMOS_TEST_SLEEP_SECONDS: sleeps for that many seconds and exits 0 instead of running
//     the test suite; used by TestKillIfCancelled_KillsRealProcess to stand in for a
//     long-running subprocess.
//   - _ATMOS_TEST_EXIT_ONE: exits immediately with code 1; stands in for a failing command
//     (e.g. `terraform show` exiting non-zero) in BuildDependencyTree tests.
//   - _ATMOS_TEST_TF_SHOW_JSON: writes its value to stdout and exits 0; stands in for
//     `terraform show -json` in BuildDependencyTree tests.
func TestMain(m *testing.M) {
	if secs := os.Getenv("_ATMOS_TEST_SLEEP_SECONDS"); secs != "" {
		if n, err := strconv.Atoi(secs); err == nil {
			time.Sleep(time.Duration(n) * time.Second)
		}
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	if planJSON := os.Getenv("_ATMOS_TEST_TF_SHOW_JSON"); planJSON != "" {
		fmt.Fprint(os.Stdout, planJSON)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
