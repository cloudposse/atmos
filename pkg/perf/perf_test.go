package perf

import (
	"sync"
	"testing"
	"time"
)

func TestTrack(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	tests := []struct {
		name          string
		functionName  string
		sleepDuration time.Duration
	}{
		{
			name:          "Track simple function",
			functionName:  "testFunc1",
			sleepDuration: 10 * time.Millisecond,
		},
		{
			name:          "Track another function",
			functionName:  "testFunc2",
			sleepDuration: 5 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := Track(nil, tt.functionName)
			time.Sleep(tt.sleepDuration)
			done()

			// Verify metric was recorded.
			reg.mu.Lock()
			metric, exists := reg.data[tt.functionName]
			reg.mu.Unlock()

			if !exists {
				t.Errorf("expected metric for %s to exist", tt.functionName)
				return
			}

			if metric.Count != 1 {
				t.Errorf("expected count 1, got %d", metric.Count)
			}

			if metric.Total < tt.sleepDuration {
				t.Errorf("expected total >= %v, got %v", tt.sleepDuration, metric.Total)
			}

			if metric.Max < tt.sleepDuration {
				t.Errorf("expected max >= %v, got %v", tt.sleepDuration, metric.Max)
			}
		})
	}
}

func TestTrackMultipleCalls(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	functionName := "multiCallFunc"
	numCalls := 5

	for i := 0; i < numCalls; i++ {
		done := Track(nil, functionName)
		time.Sleep(1 * time.Millisecond)
		done()
	}

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	if metric.Count != int64(numCalls) {
		t.Errorf("expected count %d, got %d", numCalls, metric.Count)
	}

	if metric.Total == 0 {
		t.Error("expected non-zero total time")
	}

	if metric.Max == 0 {
		t.Error("expected non-zero max time")
	}
}

func TestP95Histogram(t *testing.T) {
	// Reset registry.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	EnableTracking(true)

	functionName := "p95TestFunc"
	done := Track(nil, functionName)
	time.Sleep(1 * time.Millisecond)
	done()

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// HDR histogram should always be created when tracking is enabled.
	if metric.Hist == nil {
		t.Error("expected histogram to be created automatically")
	}
}

func TestSnapshotTop(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// Create some test data.
	functions := []struct {
		name     string
		duration time.Duration
	}{
		{"slowFunc", 100 * time.Millisecond},
		{"mediumFunc", 50 * time.Millisecond},
		{"fastFunc", 10 * time.Millisecond},
	}

	for _, fn := range functions {
		done := Track(nil, fn.name)
		time.Sleep(fn.duration)
		done()
	}

	tests := []struct {
		name     string
		sortBy   string
		topN     int
		expected int
	}{
		{
			name:     "Get all functions",
			sortBy:   "total",
			topN:     0,
			expected: 3,
		},
		{
			name:     "Get top 2 functions",
			sortBy:   "total",
			topN:     2,
			expected: 2,
		},
		{
			name:     "Sort by total",
			sortBy:   "total",
			topN:     0,
			expected: 3,
		},
		{
			name:     "Sort by avg",
			sortBy:   "avg",
			topN:     0,
			expected: 3,
		},
		{
			name:     "Sort by max",
			sortBy:   "max",
			topN:     0,
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := SnapshotTop(tt.sortBy, tt.topN)

			if len(snap.Rows) != tt.expected {
				t.Errorf("expected %d rows, got %d", tt.expected, len(snap.Rows))
			}

			if snap.TotalFuncs != 3 {
				t.Errorf("expected 3 total functions, got %d", snap.TotalFuncs)
			}

			if snap.TotalCalls != 3 {
				t.Errorf("expected 3 total calls, got %d", snap.TotalCalls)
			}

			if snap.Elapsed == 0 {
				t.Error("expected non-zero elapsed time")
			}

			// Verify sorting when we have rows.
			if len(snap.Rows) > 1 && tt.sortBy == "total" {
				for i := 0; i < len(snap.Rows)-1; i++ {
					if snap.Rows[i].Total < snap.Rows[i+1].Total {
						t.Error("rows are not sorted by total in descending order")
					}
				}
			}
		})
	}
}

func TestConcurrentTracking(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	functionName := "concurrentFunc"
	numGoroutines := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			done := Track(nil, functionName)
			time.Sleep(1 * time.Millisecond)
			done()
		}()
	}

	wg.Wait()

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	if metric.Count != int64(numGoroutines) {
		t.Errorf("expected count %d, got %d (potential race condition)", numGoroutines, metric.Count)
	}
}

func TestBuildRows(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}

	// Add test data.
	reg.data["func1"] = &Metric{
		Name:  "func1",
		Count: 10,
		Total: 100 * time.Millisecond,
		Max:   20 * time.Millisecond,
	}

	reg.data["func2"] = &Metric{
		Name:  "func2",
		Count: 5,
		Total: 50 * time.Millisecond,
		Max:   15 * time.Millisecond,
	}

	rows := buildRows()

	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}

	// Verify average calculation.
	for _, row := range rows {
		if row.Name == "func1" {
			expectedAvg := 10 * time.Millisecond // 100ms / 10
			if row.Avg != expectedAvg {
				t.Errorf("expected avg %v, got %v", expectedAvg, row.Avg)
			}
		}
		if row.Name == "func2" {
			expectedAvg := 10 * time.Millisecond // 50ms / 5
			if row.Avg != expectedAvg {
				t.Errorf("expected avg %v, got %v", expectedAvg, row.Avg)
			}
		}
	}
}

func TestSortRows(t *testing.T) {
	rows := []Row{
		{Name: "func1", Total: 100 * time.Millisecond, Avg: 10 * time.Millisecond, Max: 20 * time.Millisecond},
		{Name: "func2", Total: 50 * time.Millisecond, Avg: 25 * time.Millisecond, Max: 30 * time.Millisecond},
		{Name: "func3", Total: 75 * time.Millisecond, Avg: 15 * time.Millisecond, Max: 25 * time.Millisecond},
	}

	tests := []struct {
		name          string
		sortBy        string
		expectedFirst string
	}{
		{
			name:          "Sort by total",
			sortBy:        "total",
			expectedFirst: "func1",
		},
		{
			name:          "Sort by avg",
			sortBy:        "avg",
			expectedFirst: "func2",
		},
		{
			name:          "Sort by max",
			sortBy:        "max",
			expectedFirst: "func2",
		},
		{
			name:          "Default sort (total)",
			sortBy:        "unknown",
			expectedFirst: "func1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the original.
			testRows := make([]Row, len(rows))
			copy(testRows, rows)

			sortRows(testRows, tt.sortBy)

			if testRows[0].Name != tt.expectedFirst {
				t.Errorf("expected first row to be %s, got %s", tt.expectedFirst, testRows[0].Name)
			}
		})
	}
}

func TestP95WithHDR(t *testing.T) {
	// Reset registry and enable tracking (HDR is automatically enabled).
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	EnableTracking(true)

	functionName := "p95TestFunc"

	// Record multiple calls with varying durations.
	durations := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		3 * time.Millisecond,
		4 * time.Millisecond,
		100 * time.Millisecond, // Outlier
	}

	for _, d := range durations {
		done := Track(nil, functionName)
		time.Sleep(d)
		done()
	}

	snap := SnapshotTop("total", 0)

	if len(snap.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(snap.Rows))
	}

	row := snap.Rows[0]

	if row.P95 == 0 {
		t.Error("expected non-zero P95 when tracking is enabled")
	}

	// P95 should be greater than or equal to the average.
	if row.P95 < row.Avg {
		t.Errorf("P95 (%v) should be >= avg (%v)", row.P95, row.Avg)
	}

	// P95 may slightly exceed max due to histogram precision/rounding.
	// Allow a small tolerance (1%).
	tolerance := row.Max / 100
	if row.P95 > row.Max+tolerance {
		t.Errorf("P95 (%v) should be approximately <= max (%v) with tolerance", row.P95, row.Max)
	}
}
