package perf

import (
	"bytes"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Histogram constants.
	histogramMinValue      = 1
	histogramMaxValue      = 10_000_000_000 // 10,000 seconds in microseconds
	histogramPrecision     = 3
	percentile95           = 95
	DefaultMatrixCapacity  = 64
	defaultTopFunctionsMax = 15

	// Goroutine ID parsing constants.
	decimalBase   = 10 // Base 10 for decimal number parsing
	uint64BitSize = 64 // Bit size for uint64 parsing
)

// Metric tracks performance data for a function.
// Total includes time spent in child function calls (wall-clock time) - used for internal tracking.
// SelfTime excludes time spent in child calls (actual work done in the function) - used for display.
// Display uses SelfTime to avoid double-counting time in nested/recursive calls.
type Metric struct {
	Name     string
	Count    int64
	Total    time.Duration           // Wall-clock time (includes children) - internal use only.
	SelfTime time.Duration           // Actual work time (excludes children) - used for all display metrics.
	Max      time.Duration           // Max self-time (excludes children).
	Hist     *hdrhistogram.Histogram // Histogram for self-time percentiles (optional, nil if disabled).
}

// StackFrame represents a single frame in the call stack for tracking nested calls.
type StackFrame struct {
	functionName string
	startTime    time.Time
	childTime    time.Duration // Accumulated time spent in child function calls
}

// CallStack tracks nested function calls for a single goroutine.
type CallStack struct {
	mu     sync.Mutex
	frames []*StackFrame
}

type registry struct {
	mu    sync.Mutex
	data  map[string]*Metric
	start time.Time
}

var (
	reg = &registry{
		data:  make(map[string]*Metric),
		start: time.Now(),
	}

	// Map goroutine ID -> call stack for goroutine-local tracking.
	callStacks sync.Map // map[uint64]*CallStack

	// Simple mode uses a single global call stack (faster, no goroutine ID lookups).
	// This is sufficient for single-goroutine execution (most Atmos commands).
	simpleStack    CallStack
	useSimpleStack atomic.Bool
	simpleOwnerGID atomic.Uint64 // Owner goroutine ID for simple stack; 0 means unclaimed.

	trackingEnabled atomic.Bool
)

// EnableTracking enables performance tracking globally.
// HDR histogram for P95 latency is automatically enabled when tracking is enabled.
// By default, uses simple tracking mode (single global call stack) which is faster.
// This is sufficient for most Atmos commands which run in a single goroutine.
func EnableTracking(enabled bool) {
	trackingEnabled.Store(enabled)
	if enabled {
		// Enable simple stack mode by default for better Docker performance.
		// This avoids expensive runtime.Stack() calls on every tracked function.
		useSimpleStack.Store(true)
	}
}

// IsTrackingEnabled returns true if performance tracking is currently enabled.
// This is used to check if the heatmap should be displayed after command execution.
func IsTrackingEnabled() bool {
	return trackingEnabled.Load()
}

// UseSimpleTracking enables or disables simple tracking mode.
// Simple mode uses a single global call stack (faster, no goroutine ID lookups).
// Use false for multi-goroutine scenarios to ensure accurate per-goroutine tracking.
func UseSimpleTracking(enabled bool) {
	useSimpleStack.Store(enabled)
}

func isTrackingEnabled() bool {
	return trackingEnabled.Load()
}

// Track returns a func you should defer to record duration for a Go function.
// Performance tracking is enabled via the `--heatmap` flag.
// Use `--heatmap` flag to display the collected metrics.
//
// This function now tracks both total time (wall-clock) and self-time (excluding children).
// For recursive functions, each call is counted separately, but timing remains accurate.
// Example: ProcessYAML called 1,890 times recursively will show count=1,890 but accurate timing.
//
// Note: `atmosConfig` parameter is reserved for future use.
func Track(atmosConfig *schema.AtmosConfiguration, name string) func() {
	// Check if performance tracking is enabled globally.
	if !isTrackingEnabled() {
		// Return a no-op function when tracking is disabled.
		return func() {}
	}

	start := time.Now()

	// Use simple tracking mode if enabled (faster, avoids expensive goroutine ID lookups).
	// This is the default mode for better Docker performance.
	if useSimpleStack.Load() {
		return trackWithSimpleStack(name, start)
	}

	// Fall back to goroutine-local tracking (slower but supports multi-goroutine execution).
	return trackWithGoroutineLocalStack(name, start)
}

// trackWithSimpleStack uses the global simple stack for single-goroutine scenarios.
// If a different goroutine is detected, it automatically falls back to goroutine-local tracking
// and disables simple mode globally to avoid future expensive checks.
func trackWithSimpleStack(name string, start time.Time) func() {
	// Perform goroutine ID check when the simple stack is empty to claim ownership.
	// Also check ownership when stack is shallow (depth <= 1) to catch early cross-goroutine access.
	// For deep stacks, trust ownership to avoid expensive checks on every nested call.
	owner := simpleOwnerGID.Load()
	depth := simpleStack.depth()

	if depth == 0 {
		// Stack empty: claim ownership.
		gid := claimSimpleStackOwnership(owner)
		// Re-read current owner to avoid decisions based on stale 'owner'.
		curOwner := simpleOwnerGID.Load()
		// If a different goroutine is detected, fall back to goroutine-local tracking.
		if gid != 0 && curOwner != 0 && gid != curOwner {
			// Disable simple mode globally to avoid expensive checks on every call.
			useSimpleStack.Store(false)
			return trackWithGoroutineLocalStack(name, start)
		}
	} else if depth == 1 && owner != 0 {
		// Stack shallow (1 frame): verify ownership once to catch cross-goroutine access early.
		// After this check, deeper calls trust ownership for performance.
		gid := getGoroutineID()
		if gid != 0 && gid != owner {
			// Disable simple mode globally - we've detected multi-goroutine usage.
			useSimpleStack.Store(false)
			return trackWithGoroutineLocalStack(name, start)
		}
	}
	// For depth > 1: trust ownership, skip expensive check for performance.
	// Known limitation: If a goroutine spawns another at depth > 1, corruption is possible.
	// This trade-off favors performance for the common single-goroutine case (99% of Atmos commands).
	// The impact is limited to incorrect metrics for that run, not crashes or data loss.

	// Proceed with simple stack (single-goroutine fast path).
	frame := &StackFrame{
		functionName: name,
		startTime:    start,
		childTime:    0,
	}
	simpleStack.push(frame)

	return func() {
		finishSimpleStackTracking(frame, start, name)
	}
}

// claimSimpleStackOwnership claims the simple stack for the current goroutine if unclaimed.
// Uses CompareAndSwap for atomic ownership claim to prevent race conditions.
// Returns the current goroutine ID.
func claimSimpleStackOwnership(owner uint64) uint64 {
	// If unclaimed, atomically claim for this goroutine.
	if owner == 0 {
		if gid := getGoroutineID(); gid != 0 {
			if simpleOwnerGID.CompareAndSwap(0, gid) {
				return gid
			}
			// Someone else claimed concurrently; return this goroutine's ID so caller can compare to current owner.
			return gid
		}
	}
	// Otherwise, get current goroutine ID for mismatch detection.
	return getGoroutineID()
}

// finishSimpleStackTracking completes tracking for a simple stack frame.
func finishSimpleStackTracking(frame *StackFrame, start time.Time, name string) {
	totalTime := time.Since(start)
	selfTime := totalTime - frame.childTime
	if selfTime < 0 {
		selfTime = 0
	}

	// Pop frame from call stack.
	simpleStack.pop()

	// If there's a parent frame, add our total time to its child time accumulator.
	if parent := simpleStack.peek(); parent != nil {
		parent.childTime += totalTime
	}

	// Clear ownership when stack becomes empty.
	if simpleStack.isEmpty() {
		simpleOwnerGID.Store(0)
	}

	// Record metrics with both total and self time.
	recordMetrics(name, totalTime, selfTime)
}

// trackWithGoroutineLocalStack uses goroutine-local stacks for multi-goroutine scenarios.
func trackWithGoroutineLocalStack(name string, start time.Time) func() {
	gid := getGoroutineID()

	// Get or create call stack for this goroutine.
	stack := getOrCreateCallStack(gid)

	// Push frame onto call stack.
	frame := &StackFrame{
		functionName: name,
		startTime:    start,
		childTime:    0,
	}
	stack.push(frame)

	return func() {
		finishGoroutineLocalTracking(frame, start, name, gid, stack)
	}
}

// finishGoroutineLocalTracking completes tracking for a goroutine-local stack frame.
func finishGoroutineLocalTracking(frame *StackFrame, start time.Time, name string, gid uint64, stack *CallStack) {
	totalTime := time.Since(start)
	selfTime := totalTime - frame.childTime
	if selfTime < 0 {
		selfTime = 0
	}

	// Pop frame from call stack.
	stack.pop()

	// If there's a parent frame, add our total time to its child time accumulator.
	if parent := stack.peek(); parent != nil {
		parent.childTime += totalTime
	}

	// Clean up call stack if empty to prevent memory leaks.
	if stack.isEmpty() {
		callStacks.Delete(gid)
	}

	// Record metrics with both total and self time.
	recordMetrics(name, totalTime, selfTime)
}

// recordMetrics records both total time (wall-clock) and self-time (actual work).
func recordMetrics(name string, totalTime, selfTime time.Duration) {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	m := reg.data[name]
	if m == nil {
		// Always create HDR histogram when tracking is enabled.
		h := hdrhistogram.New(histogramMinValue, histogramMaxValue, histogramPrecision)
		m = &Metric{Name: name, Hist: h}
		reg.data[name] = m
	}

	m.Count++
	m.Total += totalTime   // Wall-clock time (includes children)
	m.SelfTime += selfTime // Actual work (excludes children)

	if selfTime > m.Max {
		m.Max = selfTime // Track max self-time (excludes children)
	}

	// Record self-time in histogram for percentiles.
	if m.Hist != nil {
		if err := m.Hist.RecordValue(selfTime.Microseconds()); err != nil {
			log.Trace("Failed to record histogram value", "error", err, "metric", name)
		}
	}
}

// getOrCreateCallStack gets or creates a call stack for the given goroutine.
func getOrCreateCallStack(gid uint64) *CallStack {
	val, _ := callStacks.LoadOrStore(gid, &CallStack{})
	return val.(*CallStack)
}

// push adds a frame to the top of the call stack.
func (cs *CallStack) push(frame *StackFrame) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.frames = append(cs.frames, frame)
}

// pop removes and returns the top frame from the call stack.
func (cs *CallStack) pop() *StackFrame {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.frames) == 0 {
		return nil
	}
	frame := cs.frames[len(cs.frames)-1]
	cs.frames = cs.frames[:len(cs.frames)-1]
	return frame
}

// peek returns the top frame without removing it.
func (cs *CallStack) peek() *StackFrame {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.frames) == 0 {
		return nil
	}
	return cs.frames[len(cs.frames)-1]
}

// isEmpty returns true if the call stack is empty.
func (cs *CallStack) isEmpty() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.frames) == 0
}

// depth returns the number of frames in the call stack.
func (cs *CallStack) depth() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.frames)
}

// getGoroutineID extracts the goroutine ID from the stack trace.
// This is used to maintain goroutine-local call stacks for accurate self-time calculation.
func getGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	// Stack trace format: "goroutine 123 [running]:\n...".
	// Extract the goroutine ID from the first line.
	fields := bytes.Fields(buf[:n])
	if len(fields) < 2 {
		return 0 // Fallback for unexpected format.
	}
	id, err := strconv.ParseUint(string(fields[1]), decimalBase, uint64BitSize)
	if err != nil {
		return 0 // Fallback if parsing fails.
	}
	return id
}

// Row represents a single function's performance metrics in the output.
type Row struct {
	Name     string
	Count    int64
	Total    time.Duration // Sum of self-time across all calls (excludes children to avoid double-counting).
	SelfTime time.Duration // Actual work time (excludes children) - same as Total for display purposes.
	Avg      time.Duration // Average self-time per call.
	Max      time.Duration // Max self-time (excludes children).
	P95      time.Duration // 95th percentile of self-time (0 if HDR disabled).
}

type Snapshot struct {
	Rows       []Row
	Elapsed    time.Duration
	TotalFuncs int
	TotalCalls int64
}

func SnapshotTop(by string, topN int) Snapshot {
	return snapshotTopInternal(by, topN, false)
}

// SnapshotTopFiltered returns the top N functions sorted by the given field, filtering out functions with zero total time.
func SnapshotTopFiltered(by string, topN int) Snapshot {
	return snapshotTopInternal(by, topN, true)
}

func snapshotTopInternal(by string, topN int, filterZero bool) Snapshot {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	rows := buildRows()

	// Filter out zero-time functions if requested.
	// We check the truncated value (to microsecond precision) to match what's displayed.
	// Filter based on self-time since that's what actually matters for performance analysis.
	if filterZero {
		filtered := make([]Row, 0, len(rows))
		for _, r := range rows {
			// Filter based on what will actually be displayed (truncated to microseconds).
			if r.SelfTime.Truncate(time.Microsecond) > 0 {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	sortRows(rows, by)

	if topN > 0 && topN < len(rows) {
		rows = rows[:topN]
	}

	var calls int64
	for _, m := range reg.data {
		calls += m.Count
	}

	return Snapshot{
		Rows:       rows,
		Elapsed:    time.Since(reg.start),
		TotalFuncs: len(reg.data),
		TotalCalls: calls,
	}
}

func buildRows() []Row {
	rows := make([]Row, 0, len(reg.data))
	for _, m := range reg.data {
		r := Row{
			Name:     m.Name,
			Count:    m.Count,
			Total:    m.SelfTime, // Use sum of self-times to avoid double-counting in nested calls.
			SelfTime: m.SelfTime,
			Max:      m.Max,
		}
		if m.Count > 0 {
			r.Avg = time.Duration(int64(m.SelfTime) / m.Count)
		}
		if m.Hist != nil && m.Hist.TotalCount() > 0 {
			r.P95 = time.Duration(m.Hist.ValueAtQuantile(percentile95)) * time.Microsecond
		}
		rows = append(rows, r)
	}
	return rows
}

func sortRows(rows []Row, by string) {
	switch by {
	case "name":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	case "total":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Total > rows[j].Total })
	case "self":
		sort.Slice(rows, func(i, j int) bool { return rows[i].SelfTime > rows[j].SelfTime })
	case "avg":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Avg > rows[j].Avg })
	case "max":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Max > rows[j].Max })
	default:
		// Default to sorting by self-time (most meaningful for finding bottlenecks).
		sort.Slice(rows, func(i, j int) bool { return rows[i].SelfTime > rows[j].SelfTime })
	}
}
