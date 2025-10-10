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

func TestSnapshotTopFiltered(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// Create test data with non-zero time functions.
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

	// Manually add zero-time functions to registry.
	reg.mu.Lock()
	reg.data["zeroFunc1"] = &Metric{Name: "zeroFunc1", Count: 1, Total: 0, Max: 0}
	reg.data["zeroFunc2"] = &Metric{Name: "zeroFunc2", Count: 2, Total: 0, Max: 0}
	reg.mu.Unlock()

	t.Run("Filter out zero-time functions", func(t *testing.T) {
		// Unfiltered should show all 5 functions.
		snapUnfiltered := SnapshotTop("total", 0)
		if len(snapUnfiltered.Rows) != 5 {
			t.Errorf("unfiltered: expected 5 rows, got %d", len(snapUnfiltered.Rows))
		}

		// Filtered should show only 3 functions with non-zero time.
		snapFiltered := SnapshotTopFiltered("total", 0)
		if len(snapFiltered.Rows) != 3 {
			t.Errorf("filtered: expected 3 rows, got %d", len(snapFiltered.Rows))
		}

		// Verify all filtered rows have non-zero total time.
		for _, r := range snapFiltered.Rows {
			if r.Total == 0 {
				t.Errorf("filtered snapshot contains zero-time function: %s", r.Name)
			}
		}

		// Verify unfiltered contains zero-time functions.
		hasZero := false
		for _, r := range snapUnfiltered.Rows {
			if r.Total == 0 {
				hasZero = true
				break
			}
		}
		if !hasZero {
			t.Error("unfiltered snapshot should contain zero-time functions")
		}
	})
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

func TestRecursiveFunctionTracking(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test verifies that recursive functions only track top-level calls.
	// This prevents count inflation when functions recurse deeply (as was happening
	// with utils.processCustomTags and exec.ProcessBaseComponentConfig).
	//
	// The correct pattern for recursive functions is:
	// 1. Public wrapper with defer perf.Track() that calls internal version
	// 2. Internal recursive function without tracking
	//
	// This ensures only top-level calls are counted, not every recursive invocation.

	functionName := "recursiveFunc"
	recursionDepth := 100 // Simulates deep YAML nesting or inheritance chains.
	topLevelCalls := 3    // Number of times the public wrapper is called.

	// Simulate calling a recursive function using the correct pattern.
	recursiveFuncPublic := func(depth int) {
		// Public wrapper tracks only this top-level call.
		done := Track(nil, functionName)
		defer done()

		// Internal recursive implementation (not tracked).
		var recursiveFuncInternal func(int)
		recursiveFuncInternal = func(d int) {
			if d > 0 {
				time.Sleep(100 * time.Microsecond) // Simulate work.
				recursiveFuncInternal(d - 1)       // Recursive call WITHOUT tracking.
			}
		}

		recursiveFuncInternal(depth)
	}

	// Call the public wrapper multiple times.
	for i := 0; i < topLevelCalls; i++ {
		recursiveFuncPublic(recursionDepth)
	}

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// CRITICAL: Count should be equal to topLevelCalls, NOT topLevelCalls * recursionDepth.
	// Before the fix, this would have been 300 (3 * 100) instead of 3.
	if metric.Count != int64(topLevelCalls) {
		t.Errorf("expected count %d (top-level calls only), got %d (indicates recursive inflation)", topLevelCalls, metric.Count)
	}

	// Verify timing is reasonable (not inflated by recursion).
	// Total time should be roughly: topLevelCalls * recursionDepth * 100µs
	expectedMinTime := time.Duration(topLevelCalls*recursionDepth*100) * time.Microsecond
	if metric.Total < expectedMinTime {
		t.Errorf("expected total >= %v, got %v", expectedMinTime, metric.Total)
	}

	// Ensure the total time isn't massively inflated (should be less than 1 second for this test).
	if metric.Total > 1*time.Second {
		t.Errorf("total time suspiciously high: %v (may indicate counting issue)", metric.Total)
	}
}

func TestRecursiveFunctionWrongPattern(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test demonstrates the WRONG pattern that causes count inflation.
	// DO NOT use this pattern - it's here to show what NOT to do.

	functionName := "badRecursiveFunc"
	recursionDepth := 10 // Smaller depth to keep test fast.
	topLevelCalls := 2

	// WRONG PATTERN: Tracking on every recursive call (BEFORE the fix).
	var badRecursiveFunc func(int)
	badRecursiveFunc = func(depth int) {
		// This tracks EVERY recursive call, inflating the count.
		done := Track(nil, functionName)
		defer done()

		if depth > 0 {
			time.Sleep(100 * time.Microsecond)
			badRecursiveFunc(depth - 1) // Recursive call WITH tracking (WRONG).
		}
	}

	// Call the function multiple times.
	for i := 0; i < topLevelCalls; i++ {
		badRecursiveFunc(recursionDepth)
	}

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// With the WRONG pattern, count is inflated by recursion depth.
	// For depth=10 and 2 calls: 2 calls * 11 levels (0-10) = 22 total tracking calls.
	expectedInflatedCount := topLevelCalls * (recursionDepth + 1)
	if metric.Count != int64(expectedInflatedCount) {
		t.Errorf("wrong pattern: expected inflated count %d, got %d", expectedInflatedCount, metric.Count)
	}

	// This demonstrates why the old pattern was problematic:
	// - Customer had ~1,890x inflation in real usage.
	// - That suggests recursion depth of ~1,890 levels in their YAML structure.
	// - With correct pattern, they would see count=1 instead of count=1,890.
}

func TestMultipleRecursiveFunctionsIndependent(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// Test that multiple recursive functions track independently.
	func1Name := "recursiveFunc1"
	func2Name := "recursiveFunc2"

	recursiveFunc1 := func(depth int) {
		done := Track(nil, func1Name)
		defer done()

		var internal func(int)
		internal = func(d int) {
			if d > 0 {
				time.Sleep(50 * time.Microsecond)
				internal(d - 1)
			}
		}
		internal(depth)
	}

	recursiveFunc2 := func(depth int) {
		done := Track(nil, func2Name)
		defer done()

		var internal func(int)
		internal = func(d int) {
			if d > 0 {
				time.Sleep(50 * time.Microsecond)
				internal(d - 1)
			}
		}
		internal(depth)
	}

	// Call each function with different parameters.
	recursiveFunc1(50) // 50 levels of recursion.
	recursiveFunc2(25) // 25 levels of recursion.

	reg.mu.Lock()
	metric1 := reg.data[func1Name]
	metric2 := reg.data[func2Name]
	reg.mu.Unlock()

	// Each function should have count=1 (one top-level call each).
	if metric1.Count != 1 {
		t.Errorf("func1: expected count 1, got %d", metric1.Count)
	}

	if metric2.Count != 1 {
		t.Errorf("func2: expected count 1, got %d", metric2.Count)
	}

	// func1 should have taken more time (deeper recursion).
	if metric1.Total <= metric2.Total {
		t.Errorf("func1 total (%v) should be > func2 total (%v) due to deeper recursion",
			metric1.Total, metric2.Total)
	}
}
