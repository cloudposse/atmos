//go:build !windows

package process

import (
	"os"
	"runtime"
	"syscall"
)

// bytesPerKiB is used to convert Linux's KiB-based ru_maxrss to bytes.
const bytesPerKiB = 1024

// collectPlatformMetrics extracts Rusage-based metrics on Unix systems.
func collectPlatformMetrics(m *ProcessMetrics, ps *os.ProcessState) {
	if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
		populateRusage(m, ru)
	}
}

// populateRusage fills the Rusage-derived fields, normalizing MaxRSS to bytes.
func populateRusage(m *ProcessMetrics, ru *syscall.Rusage) {
	switch runtime.GOOS {
	case "linux":
		m.MaxRSSBytes = ru.Maxrss * bytesPerKiB // Linux reports KiB.
	default:
		m.MaxRSSBytes = ru.Maxrss // macOS reports bytes.
	}

	m.MinorPageFaults = ru.Minflt
	m.MajorPageFaults = ru.Majflt
	m.InBlockOps = ru.Inblock
	m.OutBlockOps = ru.Oublock
	m.VolCtxSwitches = ru.Nvcsw
	m.InvolCtxSwitches = ru.Nivcsw
}
