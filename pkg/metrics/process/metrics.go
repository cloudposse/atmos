package process

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ProcessMetrics captures how much time and system resources a process (or a
// process tree) consumed. Fields not available on the current platform
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
	defer perf.Track(nil, "process.Baseline")()

	return Snapshot{
		takenAt: time.Now(),
		usage:   captureRusage(),
	}
}

// Since computes the ProcessMetrics accumulated since the Snapshot was taken.
func (s *Snapshot) Since() ProcessMetrics {
	defer perf.Track(nil, "process.Snapshot.Since")()

	m := diffRusage(&s.usage)
	m.WallTime = time.Since(s.takenAt)
	return m
}

// selfBaseline is captured once, as early as possible in the process's
// lifetime (at package load, before any command runs), so every caller of
// SelfUsageSoFar diffs against the same reference point — mirroring the
// single-baseline-per-process-lifetime pattern used elsewhere (e.g.
// pkg/proexec's exec-metadata capture).
//
//nolint:gochecknoglobals // Intentional: one self-usage baseline per process lifetime.
var selfBaseline = Baseline()

// SelfUsageSoFar returns the atmos process's own resource usage accumulated
// since package load. This measures only the atmos process itself, never any
// subprocess it spawns — see CollectFromProcessState for subprocess-tree
// usage.
func SelfUsageSoFar() ProcessMetrics {
	defer perf.Track(nil, "process.SelfUsageSoFar")()

	return selfBaseline.Since()
}

// SelfBaselineTakenAt returns the instant selfBaseline was captured (as
// early as possible in the process's lifetime). Used to compute the whole
// invocation's wall-clock time for the final aggregate summary.
func SelfBaselineTakenAt() time.Time {
	defer perf.Track(nil, "process.SelfBaselineTakenAt")()

	return selfBaseline.takenAt
}

// CollectFromProcessState extracts resource-usage metrics for a subprocess
// tree — the given cmd and all of its children — from cmd.ProcessState after
// cmd.Wait() has returned. Go's ProcessState.UserTime()/SystemTime() already
// aggregate "the exited process and its children" on every platform; on Unix,
// ProcessState.SysUsage() additionally exposes the same aggregate (via
// wait4(2)) for memory/page-fault/context-switch/block-I/O counters. Safe to
// call with a nil cmd or a nil ProcessState (e.g. the process never started):
// only WallTime is populated in that case.
func CollectFromProcessState(cmd *exec.Cmd, wallTime time.Duration) *ProcessMetrics {
	defer perf.Track(nil, "process.CollectFromProcessState")()

	m := &ProcessMetrics{WallTime: wallTime}
	if cmd == nil || cmd.ProcessState == nil {
		return m
	}

	ps := cmd.ProcessState
	m.UserCPUTime = ps.UserTime()
	m.SystemCPUTime = ps.SystemTime()
	populateSysUsage(m, ps)
	return m
}

// Combine sums two ProcessMetrics samples field-by-field, taking the max for
// MaxRSSBytes (a peak value, not an additive counter).
//
// WallTime is deliberately left at its zero value in the result: when a
// child process runs synchronously inside a parent's measured window, the
// child's wall time is already nested inside the parent's elapsed wall time,
// so summing the two would double-count. Callers combining metrics MUST set
// the result's WallTime explicitly from their own elapsed-time measurement
// (e.g. time.Since). Do not "fix" this by adding WallTime to the sum below.
func Combine(a, b ProcessMetrics) ProcessMetrics { //nolint:gocritic // hugeParam: value type is the public API (matches ProcessMetrics's other value-semantics functions); Combine is called rarely, never in a hot loop.
	defer perf.Track(nil, "process.Combine")()

	return ProcessMetrics{
		UserCPUTime:      a.UserCPUTime + b.UserCPUTime,
		SystemCPUTime:    a.SystemCPUTime + b.SystemCPUTime,
		MaxRSSBytes:      max(a.MaxRSSBytes, b.MaxRSSBytes),
		MinorPageFaults:  a.MinorPageFaults + b.MinorPageFaults,
		MajorPageFaults:  a.MajorPageFaults + b.MajorPageFaults,
		InBlockOps:       a.InBlockOps + b.InBlockOps,
		OutBlockOps:      a.OutBlockOps + b.OutBlockOps,
		VolCtxSwitches:   a.VolCtxSwitches + b.VolCtxSwitches,
		InvolCtxSwitches: a.InvolCtxSwitches + b.InvolCtxSwitches,
	}
}

// accumulatedTotal is the running total of every subprocess-tree
// ProcessMetrics sample Accumulate has ever been given, guarded by
// accumulatorMu. Used by DisplayFinalSummary to report combined resource
// usage across every subprocess spawned during the whole atmos invocation
// (e.g. every component plan in a multi-component --affected run).
//
//nolint:gochecknoglobals // Intentional: one running total per process lifetime, mirroring the selfBaseline pattern above.
var (
	accumulatorMu    sync.Mutex
	accumulatedTotal ProcessMetrics
)

// Accumulate adds m's resource usage into the process-wide running total.
// Safe for concurrent use. A nil m is a no-op.
func Accumulate(m *ProcessMetrics) {
	defer perf.Track(nil, "process.Accumulate")()

	if m == nil {
		return
	}

	accumulatorMu.Lock()
	defer accumulatorMu.Unlock()
	accumulatedTotal = Combine(accumulatedTotal, *m)
}

// AccumulatedTotal returns the running total accumulated so far via
// Accumulate. Returns the zero value if Accumulate was never called.
func AccumulatedTotal() ProcessMetrics {
	defer perf.Track(nil, "process.AccumulatedTotal")()

	accumulatorMu.Lock()
	defer accumulatorMu.Unlock()
	return accumulatedTotal
}

// metricsEnabled reports whether the local resource-usage display (ui.Info)
// is enabled per settings.metrics.enabled. Nil/unset — including a nil
// atmosConfig — means enabled (default true). This setting only gates local
// display; it never gates the Atmos Pro exec-metadata upload.
func metricsEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil || atmosConfig.Settings.Metrics.Enabled == nil {
		return true
	}
	return *atmosConfig.Settings.Metrics.Enabled
}

// DisplaySummary prints a one-line local resource-usage summary via
// ui.Info, gated by settings.metrics.enabled (default true). Label
// identifies the scope of the measurement (e.g. "Completed", "Total").
func DisplaySummary(label string, m ProcessMetrics, atmosConfig *schema.AtmosConfiguration) { //nolint:gocritic // hugeParam: value type matches ProcessMetrics's other value-semantics functions (Combine); called once per command, not in a hot loop.
	defer perf.Track(nil, "process.DisplaySummary")()

	if !metricsEnabled(atmosConfig) {
		return
	}

	msg := fmt.Sprintf("%s in %s | CPU: %s user, %s sys",
		label, formatDuration(m.WallTime), formatDuration(m.UserCPUTime), formatDuration(m.SystemCPUTime))
	if m.MaxRSSBytes > 0 {
		msg += fmt.Sprintf(" | Peak memory: %s", formatBytes(m.MaxRSSBytes))
	}
	ui.Info(msg)
}

// DisplayFinalSummary prints a single aggregate resource-usage summary at
// the very end of the whole atmos invocation: the atmos process's own usage
// combined with the accumulated usage of every subprocess spawned during the
// run (e.g. every component plan in a multi-component --affected run).
// No-ops when local display is disabled, or when no subprocess metrics were
// ever accumulated (e.g. commands with no subprocess, such as `describe
// affected`, don't need a second/duplicate line — their own self-usage is
// already reported by the normal exec-metadata path).
func DisplayFinalSummary(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(nil, "process.DisplayFinalSummary")()

	if !metricsEnabled(atmosConfig) {
		return
	}

	total := AccumulatedTotal()
	if total == (ProcessMetrics{}) {
		return
	}

	combined := Combine(SelfUsageSoFar(), total)
	combined.WallTime = time.Since(SelfBaselineTakenAt())
	DisplaySummary("Total", combined, atmosConfig)
}

// formatDuration formats a duration for human display.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// formatBytes formats bytes into a human-readable string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
