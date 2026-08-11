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
func diffRusage(baseline rusageSnapshot) ProcessMetrics {
	now := captureRusage()

	return ProcessMetrics{
		UserCPUTime:      timevalToDuration(now.rusage.Utime) - timevalToDuration(baseline.rusage.Utime),
		SystemCPUTime:    timevalToDuration(now.rusage.Stime) - timevalToDuration(baseline.rusage.Stime),
		MaxRSSBytes:      maxRSSBytes(now.rusage.Maxrss),
		MinorPageFaults:  int64(now.rusage.Minflt) - int64(baseline.rusage.Minflt),
		MajorPageFaults:  int64(now.rusage.Majflt) - int64(baseline.rusage.Majflt),
		InBlockOps:       int64(now.rusage.Inblock) - int64(baseline.rusage.Inblock),
		OutBlockOps:      int64(now.rusage.Oublock) - int64(baseline.rusage.Oublock),
		VolCtxSwitches:   int64(now.rusage.Nvcsw) - int64(baseline.rusage.Nvcsw),
		InvolCtxSwitches: int64(now.rusage.Nivcsw) - int64(baseline.rusage.Nivcsw),
	}
}

// timevalToDuration converts a syscall.Timeval to a time.Duration.
func timevalToDuration(tv syscall.Timeval) time.Duration {
	return time.Duration(tv.Sec)*time.Second + time.Duration(tv.Usec)*time.Microsecond
}

// maxRSSBytes normalizes syscall.Rusage.Maxrss to bytes. Linux reports it in
// kilobytes; Darwin (macOS) reports it in bytes already.
func maxRSSBytes(maxrss int64) int64 {
	if runtime.GOOS == "darwin" {
		return maxrss
	}
	return maxrss * 1024
}
