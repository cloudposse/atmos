package process

import (
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBaseline_SinceReportsElapsedWallTime(t *testing.T) {
	snap := Baseline()
	time.Sleep(20 * time.Millisecond)

	m := snap.Since()

	assert.GreaterOrEqual(t, m.WallTime, 15*time.Millisecond)
}

func TestSnapshot_SinceIsNonNegative(t *testing.T) {
	snap := Baseline()
	m := snap.Since()

	// CPU time deltas must never be negative even on a near-instant diff.
	assert.GreaterOrEqual(t, m.UserCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.SystemCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.WallTime, time.Duration(0))
}

func TestSnapshot_MultipleCallsIndependent(t *testing.T) {
	snap := Baseline()
	first := snap.Since()
	time.Sleep(5 * time.Millisecond)
	second := snap.Since()

	assert.Greater(t, second.WallTime, first.WallTime)
}

func TestSelfUsageSoFar_NonNegative(t *testing.T) {
	m := SelfUsageSoFar()

	assert.GreaterOrEqual(t, m.WallTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.UserCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, m.SystemCPUTime, time.Duration(0))
}

func TestSelfBaselineTakenAt_StableAcrossCalls(t *testing.T) {
	first := SelfBaselineTakenAt()
	time.Sleep(time.Millisecond)
	second := SelfBaselineTakenAt()

	assert.Equal(t, first, second, "selfBaseline is captured once at package load, not per-call")
}

func TestCollectFromProcessState(t *testing.T) {
	t.Run("nil cmd returns wall time only", func(t *testing.T) {
		m := CollectFromProcessState(nil, 42*time.Millisecond)

		require.NotNil(t, m)
		assert.Equal(t, 42*time.Millisecond, m.WallTime)
		assert.Zero(t, m.UserCPUTime)
		assert.Zero(t, m.SystemCPUTime)
	})

	t.Run("nil ProcessState (never run) returns wall time only", func(t *testing.T) {
		cmd := exec.Command("go", "version")

		m := CollectFromProcessState(cmd, 7*time.Millisecond)

		require.NotNil(t, m)
		assert.Equal(t, 7*time.Millisecond, m.WallTime)
		assert.Zero(t, m.UserCPUTime)
	})

	t.Run("real subprocess populates wall/cpu time", func(t *testing.T) {
		// "go" is guaranteed present in this dev/CI environment and is fully
		// cross-platform, unlike Unix-only binaries such as "true"/"false"/"sh"
		// (CLAUDE.md bans those in tests since they don't exist on Windows).
		cmd := exec.Command("go", "version")
		start := time.Now()
		require.NoError(t, cmd.Run())
		wall := time.Since(start)

		m := CollectFromProcessState(cmd, wall)

		require.NotNil(t, m)
		assert.Equal(t, wall, m.WallTime)
		assert.GreaterOrEqual(t, m.UserCPUTime, time.Duration(0))
		assert.GreaterOrEqual(t, m.SystemCPUTime, time.Duration(0))
		assert.GreaterOrEqual(t, m.MaxRSSBytes, int64(0))
	})
}

func TestCombine(t *testing.T) {
	a := ProcessMetrics{
		WallTime: 5 * time.Second, UserCPUTime: time.Second, SystemCPUTime: 2 * time.Second,
		MaxRSSBytes: 100, MinorPageFaults: 1, MajorPageFaults: 2,
		InBlockOps: 3, OutBlockOps: 4, VolCtxSwitches: 5, InvolCtxSwitches: 6,
	}
	b := ProcessMetrics{
		WallTime: 9 * time.Second, UserCPUTime: 3 * time.Second, SystemCPUTime: 4 * time.Second,
		MaxRSSBytes: 50, MinorPageFaults: 10, MajorPageFaults: 20,
		InBlockOps: 30, OutBlockOps: 40, VolCtxSwitches: 50, InvolCtxSwitches: 60,
	}

	got := Combine(a, b)

	assert.Zero(t, got.WallTime, "Combine must never sum WallTime — a synchronous child's wall time is already nested in the parent's")
	assert.Equal(t, 4*time.Second, got.UserCPUTime)
	assert.Equal(t, 6*time.Second, got.SystemCPUTime)
	assert.Equal(t, int64(100), got.MaxRSSBytes, "MaxRSSBytes takes the max, not the sum")
	assert.Equal(t, int64(11), got.MinorPageFaults)
	assert.Equal(t, int64(22), got.MajorPageFaults)
	assert.Equal(t, int64(33), got.InBlockOps)
	assert.Equal(t, int64(44), got.OutBlockOps)
	assert.Equal(t, int64(55), got.VolCtxSwitches)
	assert.Equal(t, int64(66), got.InvolCtxSwitches)
}

func TestCombine_MaxRSSTakesLargerOfEither(t *testing.T) {
	got := Combine(ProcessMetrics{MaxRSSBytes: 10}, ProcessMetrics{MaxRSSBytes: 999})
	assert.Equal(t, int64(999), got.MaxRSSBytes)

	got = Combine(ProcessMetrics{MaxRSSBytes: 999}, ProcessMetrics{MaxRSSBytes: 10})
	assert.Equal(t, int64(999), got.MaxRSSBytes)
}

func TestAccumulate_SumsSequentialCalls(t *testing.T) {
	before := AccumulatedTotal()

	Accumulate(&ProcessMetrics{UserCPUTime: time.Second, MaxRSSBytes: 10, MinorPageFaults: 1})
	Accumulate(&ProcessMetrics{UserCPUTime: 2 * time.Second, MaxRSSBytes: 30, MinorPageFaults: 2})

	after := AccumulatedTotal()

	assert.Equal(t, before.UserCPUTime+3*time.Second, after.UserCPUTime)
	assert.Equal(t, before.MinorPageFaults+3, after.MinorPageFaults)
	assert.GreaterOrEqual(t, after.MaxRSSBytes, int64(30))
}

func TestAccumulate_NilIsNoOp(t *testing.T) {
	before := AccumulatedTotal()

	Accumulate(nil)

	assert.Equal(t, before, AccumulatedTotal())
}

func TestAccumulate_ConcurrentSafe(t *testing.T) {
	before := AccumulatedTotal()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			Accumulate(&ProcessMetrics{UserCPUTime: time.Millisecond})
		}()
	}
	wg.Wait()

	after := AccumulatedTotal()
	assert.Equal(t, before.UserCPUTime+time.Duration(n)*time.Millisecond, after.UserCPUTime)
}

func boolPtr(b bool) *bool { return &b }

func TestMetricsEnabled_Gating(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		want        bool
	}{
		{name: "nil atmosConfig means enabled", atmosConfig: nil, want: true},
		{name: "nil Enabled pointer means enabled", atmosConfig: &schema.AtmosConfiguration{}, want: true},
		{
			name: "explicit true",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Metrics: schema.MetricsSettings{Enabled: boolPtr(true)}},
			},
			want: true,
		},
		{
			name: "explicit false",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Metrics: schema.MetricsSettings{Enabled: boolPtr(false)}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, metricsEnabled(tt.atmosConfig))
		})
	}
}

func TestDisplaySummary_GatingDoesNotPanic(t *testing.T) {
	m := ProcessMetrics{WallTime: time.Second, UserCPUTime: time.Millisecond, MaxRSSBytes: 1024}
	disabled := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{Metrics: schema.MetricsSettings{Enabled: boolPtr(false)}},
	}
	enabled := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{Metrics: schema.MetricsSettings{Enabled: boolPtr(true)}},
	}

	assert.NotPanics(t, func() { DisplaySummary("Completed", m, nil) })
	assert.NotPanics(t, func() { DisplaySummary("Completed", m, enabled) })
	assert.NotPanics(t, func() { DisplaySummary("Completed", m, disabled) })
}

func TestDisplayFinalSummary_GatingDoesNotPanic(t *testing.T) {
	disabled := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{Metrics: schema.MetricsSettings{Enabled: boolPtr(false)}},
	}

	// Disabled short-circuits before ever touching AccumulatedTotal().
	assert.NotPanics(t, func() { DisplayFinalSummary(disabled) })
	// Enabled (nil atmosConfig) exercises the AccumulatedTotal()/Combine path
	// regardless of whatever other tests in this package have already
	// accumulated.
	assert.NotPanics(t, func() { DisplayFinalSummary(nil) })
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "sub-second", d: 250 * time.Millisecond, want: "250ms"},
		{name: "zero", d: 0, want: "0ms"},
		{name: "whole seconds", d: 3 * time.Second, want: "3.0s"},
		{name: "fractional seconds", d: 1500 * time.Millisecond, want: "1.5s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatDuration(tt.d))
		})
	}
}

func TestFormatBytes(t *testing.T) {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	tests := []struct {
		name string
		b    int64
		want string
	}{
		{name: "bytes", b: 512, want: "512 B"},
		{name: "kilobytes", b: 2 * kb, want: "2.0 KB"},
		{name: "megabytes", b: 5 * mb, want: "5.0 MB"},
		{name: "gigabytes", b: 2 * gb, want: "2.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatBytes(tt.b))
		})
	}
}
