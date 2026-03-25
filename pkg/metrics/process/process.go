package process

import (
	"os/exec"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ProcessMetrics holds resource usage metrics collected from a completed subprocess.
type ProcessMetrics struct {
	// Universal (all platforms).
	WallTime      time.Duration // Elapsed real time.
	UserCPUTime   time.Duration // ProcessState.UserTime() - includes children.
	SystemCPUTime time.Duration // ProcessState.SystemTime() - includes children.
	ExitCode      int           // ProcessState.ExitCode().

	// Unix only (from syscall.Rusage via ProcessState.SysUsage()).
	MaxRSSBytes      int64 // Peak resident set size across process tree.
	MinorPageFaults  int64 // Page reclaims (soft faults).
	MajorPageFaults  int64 // Page faults (hard faults, required I/O).
	InBlockOps       int64 // Filesystem input operations.
	OutBlockOps      int64 // Filesystem output operations.
	VolCtxSwitches   int64 // Voluntary context switches (process yielded).
	InvolCtxSwitches int64 // Involuntary context switches (preempted).
}

// Collect runs the provided command and returns resource usage metrics.
// The returned error is the error from cmd.Run() - callers should check it
// for exit codes. Metrics are always returned (non-nil) even on error,
// as long as the process started.
func Collect(cmd *exec.Cmd) (*ProcessMetrics, error) {
	defer perf.Track(nil, "process.Collect")()

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	m := &ProcessMetrics{
		WallTime: elapsed,
		ExitCode: -1,
	}

	if ps := cmd.ProcessState; ps != nil {
		m.ExitCode = ps.ExitCode()
		m.UserCPUTime = ps.UserTime()
		m.SystemCPUTime = ps.SystemTime()
		collectPlatformMetrics(m, ps)
	}

	return m, err
}

// CollectFromProcessState extracts metrics from an already-completed ProcessState.
// Use this when cmd.Run() was called elsewhere and you have the ProcessState.
func CollectFromProcessState(cmd *exec.Cmd, wallTime time.Duration) *ProcessMetrics {
	defer perf.Track(nil, "process.CollectFromProcessState")()

	m := &ProcessMetrics{
		WallTime: wallTime,
		ExitCode: -1,
	}

	if cmd.ProcessState != nil {
		m.ExitCode = cmd.ProcessState.ExitCode()
		m.UserCPUTime = cmd.ProcessState.UserTime()
		m.SystemCPUTime = cmd.ProcessState.SystemTime()
		collectPlatformMetrics(m, cmd.ProcessState)
	}

	return m
}
