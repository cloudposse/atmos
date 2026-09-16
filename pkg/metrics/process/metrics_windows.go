//go:build windows

package process

import (
	"os"
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
func diffRusage(baseline *rusageSnapshot) ProcessMetrics {
	return diffRusageValues(captureRusage(), baseline)
}

// diffRusageValues computes the ProcessMetrics delta between two already
// captured samples. Split out from diffRusage so the delta arithmetic can be
// exercised with fixed inputs in tests.
func diffRusageValues(now rusageSnapshot, baseline *rusageSnapshot) ProcessMetrics {
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

// populateSysUsage is a no-op on Windows: os.ProcessState.SysUsage() returns
// nil there (no rusage equivalent), so memory/page-fault/context-switch/
// block-I/O counters are never available. CollectFromProcessState's
// UserTime()/SystemTime() values (already set by the caller) are Windows's
// only subprocess-tree signal, and even those are direct-process-only —
// GetProcessTimes (which Go's implementation calls) reports the named
// process alone, not its descendants, so a Terraform provider plugin's CPU
// time is not reflected in the Windows numbers the way it is on Unix.
func populateSysUsage(_ *ProcessMetrics, _ *os.ProcessState) {
}
