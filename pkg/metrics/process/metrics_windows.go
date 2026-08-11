//go:build windows

package process

import (
	"time"

	"golang.org/x/sys/windows"
)

// rusageSnapshot holds the raw Windows process-time sample used to compute a
// diff. Windows has no rusage equivalent, so only CPU times are captured;
// memory/IO/context-switch counters remain zero (omitted from JSON output via
// omitempty on the callers' DTOs).
type rusageSnapshot struct {
	kernelTime time.Duration
	userTime   time.Duration
}

// captureRusage samples the current process's CPU times via GetProcessTimes.
func captureRusage() rusageSnapshot {
	handle := windows.CurrentProcess()

	var creationTime, exitTime, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(handle, &creationTime, &exitTime, &kernelTime, &userTime); err != nil {
		// Best-effort: resource metrics are additive, not load-bearing.
		return rusageSnapshot{}
	}

	return rusageSnapshot{
		kernelTime: filetimeToDuration(kernelTime),
		userTime:   filetimeToDuration(userTime),
	}
}

// diffRusage computes the ProcessMetrics delta since the baseline sample.
func diffRusage(baseline rusageSnapshot) ProcessMetrics {
	now := captureRusage()

	return ProcessMetrics{
		UserCPUTime:   now.userTime - baseline.userTime,
		SystemCPUTime: now.kernelTime - baseline.kernelTime,
	}
}

// filetimeToDuration converts a Windows FILETIME (100-nanosecond intervals)
// to a time.Duration.
func filetimeToDuration(ft windows.Filetime) time.Duration {
	ticks := int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	return time.Duration(ticks) * 100 * time.Nanosecond
}
