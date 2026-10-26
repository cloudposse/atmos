//go:build unix

package process

import (
	"os"
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
// exercised with fixed inputs in tests. Counter fields are diffed via
// populateFromRusage (the same field-mapping CollectFromProcessState uses
// for subprocess-tree usage) rather than duplicating the mapping here;
// MaxRSSBytes is a peak value, not diffed, so it's taken directly from now.
func diffRusageValues(now, baseline *rusageSnapshot) ProcessMetrics {
	var nowMetrics, baselineMetrics ProcessMetrics
	populateFromRusage(&nowMetrics, &now.rusage)
	populateFromRusage(&baselineMetrics, &baseline.rusage)

	return ProcessMetrics{
		UserCPUTime:      timevalToDuration(now.rusage.Utime) - timevalToDuration(baseline.rusage.Utime),
		SystemCPUTime:    timevalToDuration(now.rusage.Stime) - timevalToDuration(baseline.rusage.Stime),
		MaxRSSBytes:      nowMetrics.MaxRSSBytes,
		MinorPageFaults:  nowMetrics.MinorPageFaults - baselineMetrics.MinorPageFaults,
		MajorPageFaults:  nowMetrics.MajorPageFaults - baselineMetrics.MajorPageFaults,
		InBlockOps:       nowMetrics.InBlockOps - baselineMetrics.InBlockOps,
		OutBlockOps:      nowMetrics.OutBlockOps - baselineMetrics.OutBlockOps,
		VolCtxSwitches:   nowMetrics.VolCtxSwitches - baselineMetrics.VolCtxSwitches,
		InvolCtxSwitches: nowMetrics.InvolCtxSwitches - baselineMetrics.InvolCtxSwitches,
	}
}

// populateFromRusage copies ru's fields into m using the canonical
// rusage-to-ProcessMetrics field mapping. Shared by diffRusageValues (self-
// usage diff path) and populateSysUsage (subprocess-tree path via
// CollectFromProcessState) so the mapping is defined exactly once.
func populateFromRusage(m *ProcessMetrics, ru *syscall.Rusage) {
	// syscall.Rusage's integer fields are int32 on Linux but already int64 on
	// Darwin, so these conversions are only redundant on the platform the
	// linter happens to run on (nolint:unconvert — needed cross-platform).
	m.MaxRSSBytes = maxRSSBytes(int64(ru.Maxrss)) //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.MinorPageFaults = int64(ru.Minflt)          //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.MajorPageFaults = int64(ru.Majflt)          //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.InBlockOps = int64(ru.Inblock)              //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.OutBlockOps = int64(ru.Oublock)             //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.VolCtxSwitches = int64(ru.Nvcsw)            //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
	m.InvolCtxSwitches = int64(ru.Nivcsw)         //nolint:unconvert // cross-platform: int32 on Linux, int64 on Darwin
}

// populateSysUsage extracts the subprocess tree's aggregate rusage from ps
// (populated by wait4(2) on Unix) into m. A nil ProcessState, or a
// ProcessState whose SysUsage() isn't a *syscall.Rusage (shouldn't happen on
// a real Unix cmd.Wait(), but kept defensive for safety), is a no-op —
// m keeps whatever CollectFromProcessState already set from
// ps.UserTime()/SystemTime().
func populateSysUsage(m *ProcessMetrics, ps *os.ProcessState) {
	if ps == nil {
		return
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return
	}
	populateFromRusage(m, ru)
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
