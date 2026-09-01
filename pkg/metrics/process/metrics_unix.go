//go:build unix

package process

import (
	"runtime"
	"syscall"
	"time"
)

// rusageSnapshot holds the raw Unix rusage sample used to compute a diff.
type rusageSnapshot struct {
	rusage syscall.Rusage
}

// captureRusage samples RUSAGE_SELF — the current process's own resource
// usage (not any child/subprocess).
func captureRusage() rusageSnapshot {
	var ru syscall.Rusage
	// Best-effort: on failure, leave ru zero-valued rather than failing the
	// caller (resource metrics are additive, not load-bearing).
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return rusageSnapshot{rusage: ru}
}

// diffRusage computes the ProcessMetrics delta since the baseline sample.
// Cumulative counters (rusage is process-lifetime-cumulative, not
// incremental) are diffed against a fresh sample taken now.
func diffRusage(baseline *rusageSnapshot) ProcessMetrics {
	now := captureRusage()
	return diffRusageValues(&now, baseline)
}

// diffRusageValues computes the ProcessMetrics delta between two already
// captured samples. Split out from diffRusage so the delta arithmetic can be
// exercised with fixed inputs in tests.
func diffRusageValues(now, baseline *rusageSnapshot) ProcessMetrics {
	// syscall.Rusage's integer fields are int32 on Linux but already int64 on
	// Darwin, so these conversions are only redundant on the platform the
	// linter happens to run on (nolint:unconvert — needed cross-platform).
	return ProcessMetrics{
		UserCPUTime:      timevalToDuration(now.rusage.Utime) - timevalToDuration(baseline.rusage.Utime),
		SystemCPUTime:    timevalToDuration(now.rusage.Stime) - timevalToDuration(baseline.rusage.Stime),
		MaxRSSBytes:      maxRSSBytes(now.rusage.Maxrss),
		MinorPageFaults:  int64(now.rusage.Minflt) - int64(baseline.rusage.Minflt),   //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
		MajorPageFaults:  int64(now.rusage.Majflt) - int64(baseline.rusage.Majflt),   //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
		InBlockOps:       int64(now.rusage.Inblock) - int64(baseline.rusage.Inblock), //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
		OutBlockOps:      int64(now.rusage.Oublock) - int64(baseline.rusage.Oublock), //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
		VolCtxSwitches:   int64(now.rusage.Nvcsw) - int64(baseline.rusage.Nvcsw),     //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
		InvolCtxSwitches: int64(now.rusage.Nivcsw) - int64(baseline.rusage.Nivcsw),   //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	}
}

// timevalToDuration converts a syscall.Timeval to a time.Duration.
func timevalToDuration(tv syscall.Timeval) time.Duration {
	return time.Duration(tv.Sec)*time.Second + time.Duration(tv.Usec)*time.Microsecond
}

// bytesPerKilobyte converts Linux's kilobyte-denominated Maxrss to bytes.
const bytesPerKilobyte = 1024

// maxRSSBytes normalizes syscall.Rusage.Maxrss to bytes. Linux reports it in
// kilobytes; Darwin (macOS) reports it in bytes already.
func maxRSSBytes(maxrss int64) int64 {
	if runtime.GOOS == "darwin" {
		return maxrss
	}
	return maxrss * bytesPerKilobyte
}
