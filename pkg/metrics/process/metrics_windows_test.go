//go:build windows

package process

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestFiletimeToDuration(t *testing.T) {
	tests := []struct {
		name string
		ft   windows.Filetime
		want time.Duration
	}{
		{
			name: "zero",
			ft:   windows.Filetime{},
			want: 0,
		},
		{
			name: "low bits only",
			ft:   windows.Filetime{LowDateTime: 10_000_000},
			want: time.Second,
		},
		{
			name: "high and low bits combined",
			ft:   windows.Filetime{LowDateTime: 1, HighDateTime: 1},
			want: time.Duration(int64(1)<<32|int64(1)) * 100 * time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, filetimeToDuration(tt.ft))
		})
	}
}

func TestDiffRusage_Windows(t *testing.T) {
	tests := []struct {
		name        string
		baseline    rusageSnapshot
		now         rusageSnapshot
		wantUserCPU time.Duration
		wantSysCPU  time.Duration
	}{
		{
			name:        "no change yields zero deltas",
			baseline:    rusageSnapshot{userTime: 5 * time.Second, kernelTime: 2 * time.Second},
			now:         rusageSnapshot{userTime: 5 * time.Second, kernelTime: 2 * time.Second},
			wantUserCPU: 0,
			wantSysCPU:  0,
		},
		{
			name:        "times advance by expected deltas",
			baseline:    rusageSnapshot{userTime: 5 * time.Second, kernelTime: 2 * time.Second},
			now:         rusageSnapshot{userTime: 8 * time.Second, kernelTime: 3500 * time.Millisecond},
			wantUserCPU: 3 * time.Second,
			wantSysCPU:  1500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseline := tt.baseline

			m := diffRusageValues(tt.now, &baseline)

			assert.Equal(t, tt.wantUserCPU, m.UserCPUTime)
			assert.Equal(t, tt.wantSysCPU, m.SystemCPUTime)
			// Windows has no rusage equivalent for these — must remain zero.
			assert.Zero(t, m.MaxRSSBytes)
			assert.Zero(t, m.MinorPageFaults)
			assert.Zero(t, m.MajorPageFaults)
		})
	}
}
