package perf

import (
	"sort"
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"

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
)

type Metric struct {
	Name  string
	Count int64
	Total time.Duration
	Max   time.Duration
	Hist  *hdrhistogram.Histogram // optional (nil if disabled)
}

type registry struct {
	mu    sync.Mutex
	data  map[string]*Metric
	start time.Time
}

var reg = &registry{
	data:  make(map[string]*Metric),
	start: time.Now(),
}

var (
	hdrEnabled      bool
	trackingEnabled bool
)

// EnableHDR enables HDR histogram tracking for P95 latency calculations.
func EnableHDR(enabled bool) {
	hdrEnabled = enabled
}

func withHDR() bool {
	return hdrEnabled
}

// EnableTracking enables performance tracking globally.
func EnableTracking(enabled bool) {
	trackingEnabled = enabled
}

func isTrackingEnabled() bool {
	return trackingEnabled
}

// Track returns a func you should defer to record duration for a Go function.
// Performance tracking is enabled via the `--heatmap` flag.
// Use `--heatmap` flag to display the collected metrics.
// Note: `atmosConfig` parameter is reserved for future use.
func Track(atmosConfig *schema.AtmosConfiguration, name string) func() {
	// Check if performance tracking is enabled globally.
	if !isTrackingEnabled() {
		// Return a no-op function when tracking is disabled.
		return func() {}
	}

	t0 := time.Now()
	return func() {
		d := time.Since(t0)
		reg.mu.Lock()
		m := reg.data[name]
		if m == nil {
			var h *hdrhistogram.Histogram
			if withHDR() {
				h = hdrhistogram.New(histogramMinValue, histogramMaxValue, histogramPrecision)
			}
			m = &Metric{Name: name, Hist: h}
			reg.data[name] = m
		}
		m.Count++
		m.Total += d
		if d > m.Max {
			m.Max = d
		}
		if m.Hist != nil {
			_ = m.Hist.RecordValue(d.Microseconds())
		}
		reg.mu.Unlock()
	}
}

type Row struct {
	Name  string
	Count int64
	Total time.Duration
	Avg   time.Duration
	Max   time.Duration
	P95   time.Duration // 0 if HDR disabled
}

type Snapshot struct {
	Rows       []Row
	Elapsed    time.Duration
	TotalFuncs int
	TotalCalls int64
}

func SnapshotTop(by string, topN int) Snapshot {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	rows := buildRows()
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
		r := Row{Name: m.Name, Count: m.Count, Total: m.Total, Max: m.Max}
		if m.Count > 0 {
			r.Avg = time.Duration(int64(m.Total) / m.Count)
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
	case "avg":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Avg > rows[j].Avg })
	case "max":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Max > rows[j].Max })
	default:
		sort.Slice(rows, func(i, j int) bool { return rows[i].Total > rows[j].Total })
	}
}
