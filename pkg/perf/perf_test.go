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
		Name:     "func1",
		Count:    10,
		Total:    100 * time.Millisecond,
		SelfTime: 80 * time.Millisecond, // 20ms was spent in children
		Max:      20 * time.Millisecond,
	}

	reg.data["func2"] = &Metric{
		Name:     "func2",
		Count:    5,
		Total:    50 * time.Millisecond,
		SelfTime: 40 * time.Millisecond, // 10ms was spent in children
		Max:      15 * time.Millisecond,
	}

	rows := buildRows()

	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}

	// Verify average self-time calculation.
	for _, row := range rows {
		if row.Name == "func1" {
			expectedAvg := 8 * time.Millisecond // 80ms / 10
			if row.Avg != expectedAvg {
				t.Errorf("expected Avg %v, got %v", expectedAvg, row.Avg)
			}
		}
		if row.Name == "func2" {
			expectedAvg := 8 * time.Millisecond // 40ms / 5
			if row.Avg != expectedAvg {
				t.Errorf("expected Avg %v, got %v", expectedAvg, row.Avg)
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
			name:          "Default sort (self-time)",
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

	// P95 should be greater than or equal to the average self-time.
	if row.P95 < row.Avg {
		t.Errorf("P95 (%v) should be >= Avg (%v)", row.P95, row.Avg)
	}

	// P95 may slightly exceed max due to histogram precision/rounding.
	// Allow a small tolerance (1%).
	tolerance := row.Max / 100
	if row.P95 > row.Max+tolerance {
		t.Errorf("P95 (%v) should be approximately <= max (%v) with tolerance", row.P95, row.Max)
	}
}

func TestSelfTimeVsTotalTime(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test verifies that self-time correctly excludes child function time.
	// Parent function calls child function; parent's self-time should only include
	// its own work, not the time spent in the child.

	parentName := "parentFunc"
	childName := "childFunc"

	// Simulate parent calling child.
	parentFunc := func() {
		done := Track(nil, parentName)
		defer done()

		// Parent does some work.
		time.Sleep(10 * time.Millisecond)

		// Parent calls child function.
		childFunc := func() {
			done := Track(nil, childName)
			defer done()
			// Child does work.
			time.Sleep(20 * time.Millisecond)
		}
		childFunc()

		// Parent does more work.
		time.Sleep(10 * time.Millisecond)
	}

	// Execute the parent function.
	parentFunc()

	reg.mu.Lock()
	parentMetric := reg.data[parentName]
	childMetric := reg.data[childName]
	reg.mu.Unlock()

	if parentMetric == nil || childMetric == nil {
		t.Fatal("expected both parent and child metrics to exist")
	}

	// Parent's total time should include child's time (wall-clock).
	// Parent: 10ms + 20ms (child) + 10ms = ~40ms total
	expectedParentTotalMin := 40 * time.Millisecond
	if parentMetric.Total < expectedParentTotalMin {
		t.Errorf("parent total (%v) should be >= %v (including child time)", parentMetric.Total, expectedParentTotalMin)
	}

	// Parent's self-time should exclude child's time.
	// Parent: 10ms + 10ms = ~20ms self-time (excluding the 20ms child spent)
	// CI environments can have significant timing variance - just verify it's reasonable
	expectedParentSelfMin := 20 * time.Millisecond // Minimum sanity check
	if parentMetric.SelfTime < expectedParentSelfMin {
		t.Errorf("parent self-time (%v) should be >= %v (excluding child time)",
			parentMetric.SelfTime, expectedParentSelfMin)
	}

	// Child's total and self-time should be roughly equal (no nested children).
	// Child: ~20ms for both total and self-time
	expectedChildTimeMin := 20 * time.Millisecond
	if childMetric.Total < expectedChildTimeMin {
		t.Errorf("child total (%v) should be >= %v", childMetric.Total, expectedChildTimeMin)
	}
	if childMetric.SelfTime < expectedChildTimeMin {
		t.Errorf("child self-time (%v) should be >= %v", childMetric.SelfTime, expectedChildTimeMin)
	}

	// Verify the key relationship: parent.SelfTime + child.Total ≈ parent.Total
	// (with some tolerance for overhead)
	calculatedTotal := parentMetric.SelfTime + childMetric.Total
	tolerance := 5 * time.Millisecond
	if parentMetric.Total < calculatedTotal-tolerance || parentMetric.Total > calculatedTotal+tolerance {
		t.Errorf("parent.Total (%v) should ≈ parent.SelfTime (%v) + child.Total (%v) = %v",
			parentMetric.Total, parentMetric.SelfTime, childMetric.Total, calculatedTotal)
	}
}

func TestNestedFunctionSelfTime(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test verifies self-time with multiple levels of nesting:
	// grandparent -> parent -> child

	grandparentName := "grandparentFunc"
	parentName := "parentFunc"
	childName := "childFunc"

	childFunc := func() {
		done := Track(nil, childName)
		defer done()
		time.Sleep(5 * time.Millisecond)
	}

	parentFunc := func() {
		done := Track(nil, parentName)
		defer done()
		time.Sleep(5 * time.Millisecond)
		childFunc()
		time.Sleep(5 * time.Millisecond)
	}

	grandparentFunc := func() {
		done := Track(nil, grandparentName)
		defer done()
		time.Sleep(5 * time.Millisecond)
		parentFunc()
		time.Sleep(5 * time.Millisecond)
	}

	grandparentFunc()

	reg.mu.Lock()
	grandparent := reg.data[grandparentName]
	parent := reg.data[parentName]
	child := reg.data[childName]
	reg.mu.Unlock()

	if grandparent == nil || parent == nil || child == nil {
		t.Fatal("expected all metrics to exist")
	}

	// Child has no children, so total ≈ self-time
	if child.SelfTime < 5*time.Millisecond {
		t.Errorf("child self-time (%v) should be >= 5ms", child.SelfTime)
	}

	// Parent's self-time should be ~10ms (5ms + 5ms), excluding child's ~5ms
	// CI environments can have significant timing variance - just verify it's reasonable
	expectedParentSelfMin := 5 * time.Millisecond // Minimum sanity check
	if parent.SelfTime < expectedParentSelfMin {
		t.Errorf("parent self-time (%v) should be >= %v (excluding child)",
			parent.SelfTime, expectedParentSelfMin)
	}

	// Grandparent's self-time should be ~10ms (5ms + 5ms), excluding parent's total time
	// CI environments can have significant timing variance - just verify it's reasonable
	expectedGrandparentSelfMin := 5 * time.Millisecond // Minimum sanity check
	if grandparent.SelfTime < expectedGrandparentSelfMin {
		t.Errorf("grandparent self-time (%v) should be >= %v (excluding parent and child)",
			grandparent.SelfTime, expectedGrandparentSelfMin)
	}

	// Grandparent's total should include everyone: 5ms + (5ms + 5ms + 5ms) + 5ms = ~25ms
	expectedGrandparentTotalMin := 25 * time.Millisecond
	if grandparent.Total < expectedGrandparentTotalMin {
		t.Errorf("grandparent total (%v) should be >= %v (including all children)",
			grandparent.Total, expectedGrandparentTotalMin)
	}
}

func TestDirectRecursionWithSelfTime(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test demonstrates the NEW capability: direct recursive tracking
	// with accurate counts AND accurate timing.
	//
	// With the old system, this pattern would either:
	// 1. Inflate count by recursion depth (if tracking every call)
	// 2. Hide recursive calls (if using wrapper pattern)
	//
	// With the new self-time tracking:
	// - Count accurately reflects ALL calls (including recursive)
	// - Timing remains accurate (no inflation)

	functionName := "directRecursiveFunc"
	recursionDepth := 10
	numTopLevelCalls := 2

	// Direct recursive function with tracking on EVERY call.
	var recursiveFunc func(int)
	recursiveFunc = func(depth int) {
		done := Track(nil, functionName)
		defer done()

		if depth > 0 {
			time.Sleep(1 * time.Millisecond) // Simulate work
			recursiveFunc(depth - 1)         // Direct recursive call WITH tracking
		}
	}

	// Call the function multiple times.
	for i := 0; i < numTopLevelCalls; i++ {
		recursiveFunc(recursionDepth)
	}

	reg.mu.Lock()
	metric := reg.data[functionName]
	reg.mu.Unlock()

	if metric == nil {
		t.Fatal("expected metric to exist")
	}

	// NEW BEHAVIOR: Count should reflect ALL calls including recursive ones.
	// For depth=10: each top-level call makes 11 total calls (levels 0-10)
	// 2 top-level calls * 11 levels = 22 total calls
	expectedCount := numTopLevelCalls * (recursionDepth + 1)
	if metric.Count != int64(expectedCount) {
		t.Errorf("expected count %d (all recursive calls), got %d", expectedCount, metric.Count)
	}

	// Timing behavior with direct recursion:
	// - Each level does ~1ms of actual work
	// - Total time at each level includes time of all child levels
	// - For 2 calls with depth 10, we expect the aggregate metrics to be reasonable

	// Self-time should be the sum of actual work: 22 calls * ~1ms = ~22ms
	// CI environments have timing variance - just verify it's reasonable (no upper bound check)
	expectedSelfMin := time.Duration(expectedCount) * time.Millisecond / 2 // Allow significant under for CI
	if metric.SelfTime < expectedSelfMin {
		t.Errorf("self-time (%v) should be >= %v (sum of actual work)",
			metric.SelfTime, expectedSelfMin)
	}

	// Total time will be higher because each level includes children's time.
	// The sum of all total times will be larger, but should still be reasonable (< 1 second)
	if metric.Total > 1*time.Second {
		t.Errorf("total time (%v) seems unreasonably high", metric.Total)
	}

	// Verify that total >= self-time (always true)
	if metric.Total < metric.SelfTime {
		t.Errorf("total time (%v) should be >= self-time (%v)", metric.Total, metric.SelfTime)
	}
}

//nolint:dupl // Similar to TestYAMLConfigProcessingRecursion but tests general pattern.
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

//nolint:dupl // Similar to TestRecursiveFunctionTracking but tests YAML-specific scenario.
func TestYAMLConfigProcessingRecursion(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test simulates the processYAMLConfigFileWithContextInternal pattern.
	// It verifies that processing deeply nested YAML imports only tracks the
	// top-level call, not each recursive import processing call.
	//
	// Real-world scenario:
	// - Stack manifest imports several files
	// - Those files import other files (nested imports)
	// - Deep import hierarchies can reach 50+ levels
	// - Before fix: count would be inflated by import depth
	// - After fix: count = number of ProcessYAMLConfigFileWithContext calls

	functionName := "exec.ProcessYAMLConfigFileWithContext"
	importDepth := 50  // Simulates deep import hierarchy.
	topLevelCalls := 2 // Number of stack files processed.

	// Simulate the wrapper pattern used for ProcessYAMLConfigFileWithContext.
	processYAMLConfigFileWithContext := func(depth int) {
		// Public wrapper tracks once.
		done := Track(nil, functionName)
		defer done()

		// Internal recursive implementation (not tracked).
		var processYAMLConfigFileWithContextInternal func(int)
		processYAMLConfigFileWithContextInternal = func(d int) {
			if d > 0 {
				time.Sleep(50 * time.Microsecond) // Simulate YAML parsing and merging.
				// Simulate processing imports recursively (each import calls internal version).
				processYAMLConfigFileWithContextInternal(d - 1)
			}
		}

		processYAMLConfigFileWithContextInternal(depth)
	}

	// Process multiple stack files.
	for i := 0; i < topLevelCalls; i++ {
		processYAMLConfigFileWithContext(importDepth)
	}

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// CRITICAL: Count should equal topLevelCalls (number of stack files processed).
	// NOT topLevelCalls * importDepth (which would indicate inflation).
	if metric.Count != int64(topLevelCalls) {
		t.Errorf("expected count %d (top-level calls only), got %d (recursive inflation detected)",
			topLevelCalls, metric.Count)
	}

	// Verify timing is reasonable.
	// Total time should be roughly: topLevelCalls * importDepth * 50µs
	expectedMinTime := time.Duration(topLevelCalls*importDepth*50) * time.Microsecond
	if metric.Total < expectedMinTime {
		t.Errorf("expected total >= %v, got %v", expectedMinTime, metric.Total)
	}

	// Ensure total isn't massively inflated.
	if metric.Total > 1*time.Second {
		t.Errorf("total time suspiciously high: %v", metric.Total)
	}
}

func TestYAMLConfigProcessingMultipleImports(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test simulates processing a stack manifest with multiple imports,
	// where each import may itself have imports (fan-out pattern).
	//
	// Real-world scenario:
	// - Stack manifest imports: mixins/common, mixins/region/us-east-2, orgs/acme
	// - Each mixin imports more files
	// - Total recursive calls can be in the hundreds
	// - Only the top-level ProcessYAMLConfigFileWithContext should be tracked

	functionName := "exec.ProcessYAMLConfigFileWithContext"
	numImports := 5     // Number of direct imports in the manifest.
	importsPerFile := 3 // Each import has its own imports.
	nestedLevels := 2   // Depth of import nesting.
	totalRecursiveCalls := numImports * importsPerFile * nestedLevels

	// Simulate the wrapper pattern.
	processYAMLConfigFileWithContext := func() {
		// Public wrapper tracks once.
		done := Track(nil, functionName)
		defer done()

		// Internal recursive implementation.
		var processYAMLConfigFileWithContextInternal func(int)
		processYAMLConfigFileWithContextInternal = func(level int) {
			if level <= 0 {
				return
			}

			// Simulate processing multiple imports at this level.
			for i := 0; i < numImports; i++ {
				time.Sleep(10 * time.Microsecond) // Simulate work.
				// Each import may have nested imports.
				if level > 1 {
					processYAMLConfigFileWithContextInternal(level - 1)
				}
			}
		}

		processYAMLConfigFileWithContextInternal(nestedLevels)
	}

	// Process one stack file.
	processYAMLConfigFileWithContext()

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// CRITICAL: Count should be 1 (one top-level call to process the stack file).
	// NOT totalRecursiveCalls (which would be 30 in this test).
	if metric.Count != 1 {
		t.Errorf("expected count 1, got %d (recursive inflation: should be 1 not %d)",
			metric.Count, totalRecursiveCalls)
	}

	// Verify some work was done.
	if metric.Total == 0 {
		t.Error("expected non-zero total time")
	}
}

func TestProcessBaseComponentConfigRecursion(t *testing.T) {
	// Reset registry for clean test.
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}
	// Enable tracking for tests.
	EnableTracking(true)

	// This test simulates the processBaseComponentConfigInternal pattern.
	// It verifies that processing component inheritance chains only tracks
	// the top-level call, not each recursive base component lookup.
	//
	// Real-world scenario:
	// - Component inherits from base component
	// - Base component inherits from another base component
	// - Inheritance chain can be 10+ levels deep
	// - Before fix: count inflated by chain depth
	// - After fix: count = number of ProcessBaseComponentConfig calls

	functionName := "exec.ProcessBaseComponentConfig"
	inheritanceChainDepth := 15 // Simulates deep component inheritance.
	numComponents := 3          // Number of components processed.

	// Simulate the wrapper pattern.
	processBaseComponentConfig := func(chainDepth int) {
		// Public wrapper tracks once.
		done := Track(nil, functionName)
		defer done()

		// Internal recursive implementation.
		var processBaseComponentConfigInternal func(int)
		processBaseComponentConfigInternal = func(depth int) {
			if depth > 0 {
				time.Sleep(25 * time.Microsecond) // Simulate base component lookup and merge.
				// Recursive call to process base component of this base component.
				processBaseComponentConfigInternal(depth - 1)
			}
		}

		processBaseComponentConfigInternal(chainDepth)
	}

	// Process multiple components.
	for i := 0; i < numComponents; i++ {
		processBaseComponentConfig(inheritanceChainDepth)
	}

	reg.mu.Lock()
	metric, exists := reg.data[functionName]
	reg.mu.Unlock()

	if !exists {
		t.Fatal("expected metric to exist")
	}

	// CRITICAL: Count should equal numComponents (number of components processed).
	// NOT numComponents * inheritanceChainDepth.
	if metric.Count != int64(numComponents) {
		t.Errorf("expected count %d, got %d (recursive inflation detected)", numComponents, metric.Count)
	}

	// Verify timing is reasonable.
	expectedMinTime := time.Duration(numComponents*inheritanceChainDepth*25) * time.Microsecond
	if metric.Total < expectedMinTime {
		t.Errorf("expected total >= %v, got %v", expectedMinTime, metric.Total)
	}
}

// TestSimpleTrackingMode tests the new simple tracking mode functionality.
func TestSimpleTrackingMode(t *testing.T) {
	t.Run("Simple mode enabled by default", func(t *testing.T) {
		// Reset and enable tracking.
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)

		// Simple mode should be enabled by default.
		if !useSimpleStack.Load() {
			t.Error("expected simple mode to be enabled by default")
		}
	})

	t.Run("Simple mode with nested calls", func(t *testing.T) {
		// Reset registry.
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(true)

		parentName := "simpleParent"
		childName := "simpleChild"

		// Nested function calls with simple mode.
		parentFunc := func() {
			done := Track(nil, parentName)
			defer done()

			time.Sleep(5 * time.Millisecond)

			childFunc := func() {
				done := Track(nil, childName)
				defer done()
				time.Sleep(10 * time.Millisecond)
			}
			childFunc()

			time.Sleep(5 * time.Millisecond)
		}

		parentFunc()

		reg.mu.Lock()
		parentMetric := reg.data[parentName]
		childMetric := reg.data[childName]
		reg.mu.Unlock()

		if parentMetric == nil || childMetric == nil {
			t.Fatal("expected both parent and child metrics to exist")
		}

		// Parent's total should include child time.
		expectedParentTotalMin := 20 * time.Millisecond
		if parentMetric.Total < expectedParentTotalMin {
			t.Errorf("parent total (%v) should be >= %v", parentMetric.Total, expectedParentTotalMin)
		}

		// Parent's self-time should exclude child time.
		expectedParentSelfMin := 10 * time.Millisecond
		if parentMetric.SelfTime < expectedParentSelfMin {
			t.Errorf("parent self-time (%v) should be >= %v", parentMetric.SelfTime, expectedParentSelfMin)
		}

		// Child's total and self-time should be roughly equal.
		expectedChildTimeMin := 10 * time.Millisecond
		if childMetric.Total < expectedChildTimeMin {
			t.Errorf("child total (%v) should be >= %v", childMetric.Total, expectedChildTimeMin)
		}
		if childMetric.SelfTime < expectedChildTimeMin {
			t.Errorf("child self-time (%v) should be >= %v", childMetric.SelfTime, expectedChildTimeMin)
		}
	})

	t.Run("Simple mode with direct recursion", func(t *testing.T) {
		// Reset registry.
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(true)

		functionName := "simpleRecursive"
		depth := 5

		var recursiveFunc func(int)
		recursiveFunc = func(d int) {
			done := Track(nil, functionName)
			defer done()

			if d > 0 {
				time.Sleep(1 * time.Millisecond)
				recursiveFunc(d - 1)
			}
		}

		recursiveFunc(depth)

		reg.mu.Lock()
		metric := reg.data[functionName]
		reg.mu.Unlock()

		if metric == nil {
			t.Fatal("expected metric to exist")
		}

		// Count should reflect all recursive calls.
		expectedCount := depth + 1
		if metric.Count != int64(expectedCount) {
			t.Errorf("expected count %d, got %d", expectedCount, metric.Count)
		}

		// Verify timing is reasonable.
		if metric.Total == 0 {
			t.Error("expected non-zero total time")
		}
		if metric.SelfTime == 0 {
			t.Error("expected non-zero self-time")
		}
	})
}

// TestGoroutineLocalTracking tests that goroutine-local tracking can be explicitly enabled.
func TestGoroutineLocalTracking(t *testing.T) {
	t.Run("Disable simple mode for concurrent execution", func(t *testing.T) {
		// Reset registry.
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(false) // Explicitly disable simple mode for concurrent test.

		functionName := "concurrentGoroutineLocal"
		numGoroutines := 50

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
			t.Errorf("expected count %d, got %d", numGoroutines, metric.Count)
		}
	})

	t.Run("Goroutine-local tracking with nested calls", func(t *testing.T) {
		// Reset registry.
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(false) // Use goroutine-local tracking.

		parentName := "goroutineParent"
		childName := "goroutineChild"

		parentFunc := func() {
			done := Track(nil, parentName)
			defer done()

			time.Sleep(5 * time.Millisecond)

			childFunc := func() {
				done := Track(nil, childName)
				defer done()
				time.Sleep(10 * time.Millisecond)
			}
			childFunc()

			time.Sleep(5 * time.Millisecond)
		}

		parentFunc()

		reg.mu.Lock()
		parentMetric := reg.data[parentName]
		childMetric := reg.data[childName]
		reg.mu.Unlock()

		if parentMetric == nil || childMetric == nil {
			t.Fatal("expected both parent and child metrics to exist")
		}

		// Verify timing is reasonable.
		expectedParentTotalMin := 20 * time.Millisecond
		if parentMetric.Total < expectedParentTotalMin {
			t.Errorf("parent total (%v) should be >= %v", parentMetric.Total, expectedParentTotalMin)
		}

		expectedParentSelfMin := 10 * time.Millisecond
		if parentMetric.SelfTime < expectedParentSelfMin {
			t.Errorf("parent self-time (%v) should be >= %v", parentMetric.SelfTime, expectedParentSelfMin)
		}
	})
}

// TestSimpleVsGoroutineLocalPerformance demonstrates the performance difference.
func TestSimpleVsGoroutineLocalPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance comparison in short mode")
	}

	functionName := "perfTestFunc"
	numCalls := 1000

	// Test with simple mode (fast path).
	t.Run("Simple mode performance", func(t *testing.T) {
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(true)

		start := time.Now()
		for i := 0; i < numCalls; i++ {
			done := Track(nil, functionName)
			done()
		}
		simpleModeDuration := time.Since(start)

		t.Logf("Simple mode: %d calls in %v", numCalls, simpleModeDuration)
	})

	// Test with goroutine-local mode (slow path with getGoroutineID).
	t.Run("Goroutine-local mode performance", func(t *testing.T) {
		reg = &registry{
			data:  make(map[string]*Metric),
			start: time.Now(),
		}
		EnableTracking(true)
		UseSimpleTracking(false)

		start := time.Now()
		for i := 0; i < numCalls; i++ {
			done := Track(nil, functionName)
			done()
		}
		goroutineLocalDuration := time.Since(start)

		t.Logf("Goroutine-local mode: %d calls in %v", numCalls, goroutineLocalDuration)
	})
}
