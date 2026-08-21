//go:build windows

package process

import "os"

// collectPlatformMetrics is a no-op on Windows.
// Windows syscall.Rusage does not expose Unix fields (Maxrss, Minflt, etc.).
// Only timing metrics (WallTime, UserCPUTime, SystemCPUTime) are available.
func collectPlatformMetrics(_ *ProcessMetrics, _ *os.ProcessState) {
	// No-op: Rusage fields remain zero-valued.
}
