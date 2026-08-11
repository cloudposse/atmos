package process

import "time"

// ProcessMetrics captures how much time and system resources the atmos
// process itself consumed. Fields not available on the current platform
// (e.g. Windows) are left zero-valued; callers that marshal this struct
// should use omitempty for those fields.
type ProcessMetrics struct {
	WallTime         time.Duration
	UserCPUTime      time.Duration
	SystemCPUTime    time.Duration
	MaxRSSBytes      int64
	MinorPageFaults  int64
	MajorPageFaults  int64
	InBlockOps       int64
	OutBlockOps      int64
	VolCtxSwitches   int64
	InvolCtxSwitches int64
}

// Snapshot is a point-in-time capture of process resource usage, taken as
// early as possible in the process lifetime, used later to compute a diff
// via Since().
type Snapshot struct {
	takenAt time.Time
	usage   rusageSnapshot
}

// Baseline captures a Snapshot as early as possible in the process's
// lifetime. Both the async and synchronous exec-metadata capture paths diff
// against the same baseline.
func Baseline() Snapshot {
	return Snapshot{
		takenAt: time.Now(),
		usage:   captureRusage(),
	}
}

// Since computes the ProcessMetrics accumulated since the Snapshot was taken.
func (s Snapshot) Since() ProcessMetrics {
	m := diffRusage(s.usage)
	m.WallTime = time.Since(s.takenAt)
	return m
}
