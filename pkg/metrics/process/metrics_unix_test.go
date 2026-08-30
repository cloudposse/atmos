//go:build unix

package process

import (
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimevalToDuration(t *testing.T) {
	tests := []struct {
		name string
		tv   syscall.Timeval
		want time.Duration
	}{
		{
			name: "zero",
			tv:   syscall.Timeval{},
			want: 0,
		},
		{
			name: "seconds only",
			tv:   syscall.Timeval{Sec: 2},
			want: 2 * time.Second,
		},
		{
			name: "microseconds only",
			tv:   syscall.Timeval{Usec: 500},
			want: 500 * time.Microsecond,
		},
		{
			name: "seconds and microseconds",
			tv:   syscall.Timeval{Sec: 1, Usec: 250},
			want: time.Second + 250*time.Microsecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, timevalToDuration(tt.tv))
		})
	}
}

func TestMaxRSSBytes(t *testing.T) {
	tests := []struct {
		name   string
		maxrss int64
	}{
		{name: "zero", maxrss: 0},
		{name: "positive", maxrss: 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxRSSBytes(tt.maxrss)

			// maxRSSBytes normalizes by GOOS at runtime: Darwin reports bytes
			// already, Linux (and other non-Darwin unix) reports kilobytes.
			// Assert against the same rule the implementation uses rather
			// than hardcoding an expectation for one platform.
			want := tt.maxrss
			if runtime.GOOS != "darwin" {
				want *= bytesPerKilobyte
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestDiffRusage_CounterDeltas(t *testing.T) {
	tests := []struct {
		name     string
		baseline syscall.Rusage
		now      syscall.Rusage
		want     ProcessMetrics
	}{
		{
			name:     "no change yields zero deltas",
			baseline: syscall.Rusage{Minflt: 10, Majflt: 2, Inblock: 5, Oublock: 3, Nvcsw: 7, Nivcsw: 1},
			now:      syscall.Rusage{Minflt: 10, Majflt: 2, Inblock: 5, Oublock: 3, Nvcsw: 7, Nivcsw: 1},
			want:     ProcessMetrics{},
		},
		{
			name:     "counters increase by expected deltas",
			baseline: syscall.Rusage{Minflt: 10, Majflt: 2, Inblock: 5, Oublock: 3, Nvcsw: 7, Nivcsw: 1},
			now:      syscall.Rusage{Minflt: 25, Majflt: 4, Inblock: 9, Oublock: 6, Nvcsw: 12, Nivcsw: 3},
			want: ProcessMetrics{
				MinorPageFaults:  15,
				MajorPageFaults:  2,
				InBlockOps:       4,
				OutBlockOps:      3,
				VolCtxSwitches:   5,
				InvolCtxSwitches: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline := rusageSnapshot{rusage: tt.baseline}

			m := diffRusageValues(rusageSnapshot{rusage: tt.now}, &baseline)

			assert.Equal(t, tt.want.MinorPageFaults, m.MinorPageFaults)
			assert.Equal(t, tt.want.MajorPageFaults, m.MajorPageFaults)
			assert.Equal(t, tt.want.InBlockOps, m.InBlockOps)
			assert.Equal(t, tt.want.OutBlockOps, m.OutBlockOps)
			assert.Equal(t, tt.want.VolCtxSwitches, m.VolCtxSwitches)
			assert.Equal(t, tt.want.InvolCtxSwitches, m.InvolCtxSwitches)
		})
	}
}

func TestDiffRusage_CPUTimeDeltas(t *testing.T) {
	baseline := rusageSnapshot{
		rusage: syscall.Rusage{
			Utime: syscall.Timeval{Sec: 1, Usec: 0},
			Stime: syscall.Timeval{Sec: 0, Usec: 500},
		},
	}
	now := rusageSnapshot{
		rusage: syscall.Rusage{
			Utime: syscall.Timeval{Sec: 2, Usec: 0},
			Stime: syscall.Timeval{Sec: 0, Usec: 900},
		},
	}

	m := diffRusageValues(now, &baseline)

	assert.Equal(t, time.Second, m.UserCPUTime)
	assert.Equal(t, 400*time.Microsecond, m.SystemCPUTime)
}
