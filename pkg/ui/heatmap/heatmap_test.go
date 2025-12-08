package heatmap

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/perf"
)

func TestNewHeatModel(t *testing.T) {
	hm := NewHeatModel()

	assert.NotNil(t, hm)
	assert.NotNil(t, hm.stepIndex)
	assert.Equal(t, len(Steps), len(hm.matrix))
	assert.Equal(t, 0, hm.runCount)

	// Verify all steps are indexed.
	for i, s := range Steps {
		assert.Equal(t, i, hm.stepIndex[s])
	}
}

func TestHeatModel_AddRun(t *testing.T) {
	hm := NewHeatModel()

	sample := RunSample{
		RunIndex: 0,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 100 * time.Millisecond,
			StepLoadStacks:  200 * time.Millisecond,
		},
	}

	hm.AddRun(sample)

	assert.Equal(t, 1, hm.runCount)
	assert.Equal(t, float64(100), hm.matrix[hm.stepIndex[StepParseConfig]][0])
	assert.Equal(t, float64(200), hm.matrix[hm.stepIndex[StepLoadStacks]][0])
}

func TestHeatModel_AddRun_MultipleRuns(t *testing.T) {
	hm := NewHeatModel()

	// Add first run.
	hm.AddRun(RunSample{
		RunIndex: 0,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 100 * time.Millisecond,
		},
	})

	// Add second run.
	hm.AddRun(RunSample{
		RunIndex: 1,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 150 * time.Millisecond,
		},
	})

	assert.Equal(t, 2, hm.runCount)
	assert.Equal(t, float64(100), hm.matrix[hm.stepIndex[StepParseConfig]][0])
	assert.Equal(t, float64(150), hm.matrix[hm.stepIndex[StepParseConfig]][1])
}

func TestHeatModel_Normalized_EmptyMatrix(t *testing.T) {
	hm := NewHeatModel()

	// Add run with zero durations.
	hm.AddRun(RunSample{
		RunIndex: 0,
		StepDur:  map[Step]time.Duration{},
	})

	norm, minV, maxV := hm.Normalized()

	assert.NotNil(t, norm)
	assert.Equal(t, float64(0), minV)
	assert.Equal(t, float64(0), maxV)
}

func TestHeatModel_Normalized_SingleValue(t *testing.T) {
	hm := NewHeatModel()

	hm.AddRun(RunSample{
		RunIndex: 0,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 100 * time.Millisecond,
		},
	})

	norm, minV, maxV := hm.Normalized()

	assert.NotNil(t, norm)
	assert.Equal(t, float64(100), minV)
	assert.Equal(t, float64(100), maxV)

	// When min==max, normalized value should be 1 (full intensity).
	parseConfigIdx := hm.stepIndex[StepParseConfig]
	assert.Equal(t, float64(1), norm[parseConfigIdx][0])
}

func TestHeatModel_Normalized_MultipleValues(t *testing.T) {
	hm := NewHeatModel()

	hm.AddRun(RunSample{
		RunIndex: 0,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 100 * time.Millisecond,
			StepLoadStacks:  200 * time.Millisecond,
		},
	})

	norm, minV, maxV := hm.Normalized()

	assert.NotNil(t, norm)
	assert.Equal(t, float64(100), minV)
	assert.Equal(t, float64(200), maxV)

	parseConfigIdx := hm.stepIndex[StepParseConfig]
	loadStacksIdx := hm.stepIndex[StepLoadStacks]

	// 100ms normalized should be 0 (minimum).
	assert.Equal(t, float64(0), norm[parseConfigIdx][0])
	// 200ms normalized should be 1 (maximum).
	assert.Equal(t, float64(1), norm[loadStacksIdx][0])
}

func TestHeatModel_Normalized_ZeroDurationCells(t *testing.T) {
	hm := NewHeatModel()

	hm.AddRun(RunSample{
		RunIndex: 0,
		StepDur: map[Step]time.Duration{
			StepParseConfig: 100 * time.Millisecond,
			StepLoadStacks:  0, // Zero duration
		},
	})

	norm, _, _ := hm.Normalized()

	loadStacksIdx := hm.stepIndex[StepLoadStacks]
	// Zero duration cells should remain 0 (skipped).
	assert.Equal(t, float64(0), norm[loadStacksIdx][0])
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "Zero duration",
			duration: 0,
			expected: "0",
		},
		{
			name:     "Microseconds",
			duration: 123 * time.Microsecond,
			expected: "123µs",
		},
		{
			name:     "Milliseconds",
			duration: 456 * time.Millisecond,
			expected: "456ms",
		},
		{
			name:     "Seconds",
			duration: 2 * time.Second,
			expected: "2s",
		},
		{
			name:     "Sub-microsecond (truncated to 0)",
			duration: 500 * time.Nanosecond,
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "String shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "String equal to max",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "String longer than max",
			input:    "hello world",
			maxLen:   8,
			expected: "hello w…",
		},
		{
			name:     "Max length 1",
			input:    "hello",
			maxLen:   1,
			expected: "h",
		},
		{
			name:     "Empty string",
			input:    "",
			maxLen:   5,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase word",
			input:    "hello",
			expected: "Hello",
		},
		{
			name:     "Already capitalized",
			input:    "World",
			expected: "World",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Single character",
			input:    "a",
			expected: "A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBarColorGradient(t *testing.T) {
	colors := getBarColorGradient()

	assert.NotNil(t, colors)
	assert.Equal(t, 8, len(colors))
	// Verify it starts with red and ends with green.
	assert.Contains(t, string(colors[0]), "196") // Red
	assert.Contains(t, string(colors[7]), "118") // Green
}

func TestGetColorForPosition(t *testing.T) {
	colors := getBarColorGradient()

	tests := []struct {
		name       string
		position   int
		totalItems int
		expected   int // Expected color index
	}{
		{
			name:       "First item (slowest)",
			position:   0,
			totalItems: 8,
			expected:   0,
		},
		{
			name:       "Last item (fastest)",
			position:   7,
			totalItems: 8,
			expected:   7,
		},
		{
			name:       "Middle item",
			position:   4,
			totalItems: 8,
			expected:   4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := getColorForPosition(tt.position, tt.totalItems, colors)
			assert.Equal(t, colors[tt.expected], color)
		})
	}
}

func TestModel_GetLimitedSnapshot(t *testing.T) {
	hm := NewHeatModel()
	m := &model{
		heatModel: hm,
		initialSnap: perf.Snapshot{
			Rows: make([]perf.Row, 30), // More than topFunctionsVisualLimit (25)
		},
	}

	snap := m.getLimitedSnapshot()

	// Should be limited to topFunctionsVisualLimit.
	assert.Equal(t, topFunctionsVisualLimit, len(snap.Rows))
}

func TestModel_FindMaxTotal(t *testing.T) {
	m := &model{}

	rows := []perf.Row{
		{Total: 100 * time.Millisecond},
		{Total: 500 * time.Millisecond},
		{Total: 200 * time.Millisecond},
	}

	maxTotal := m.findMaxTotal(rows)

	assert.Equal(t, 500*time.Millisecond, maxTotal)
}

func TestModel_Init(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())

	cmd := m.Init()

	assert.NotNil(t, cmd)
	// Verify snapshot was captured.
	assert.NotNil(t, m.initialSnap)
}

func TestModel_UpdatePerformanceData(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())

	// Set up initial snapshot with test data.
	m.initialSnap = perf.Snapshot{
		Rows: []perf.Row{
			{
				Name:  "test.Function",
				Count: 10,
				Total: 100 * time.Millisecond,
				Avg:   10 * time.Millisecond,
				Max:   20 * time.Millisecond,
				P95:   15 * time.Millisecond,
			},
		},
	}

	m.updatePerformanceData()

	// Verify table was populated.
	assert.Len(t, m.table.Rows(), 1)
	assert.Contains(t, m.table.Rows()[0][0], "Function")
}

func TestModel_View(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())
	m.width = 80
	m.height = 40

	// Set up initial snapshot.
	m.initialSnap = perf.Snapshot{
		Rows: []perf.Row{
			{
				Name:  "test.Function",
				Count: 5,
				Total: 50 * time.Millisecond,
				Avg:   10 * time.Millisecond,
			},
		},
		TotalFuncs: 1,
		TotalCalls: 5,
		Elapsed:    100 * time.Millisecond,
	}

	result := m.View()

	assert.Contains(t, result, "Atmos Performance Results")
	assert.Contains(t, result, "Bar Mode")
	assert.Contains(t, result, "Functions: 1")
	assert.Contains(t, result, "Total Calls: 5")
	// Verify legend is displayed.
	assert.Contains(t, result, "Parallelism:")
	assert.Contains(t, result, "Count: # calls")
	assert.Contains(t, result, "CPU Time: sum of self-time")
	assert.Contains(t, result, "self-time")
}

func TestModel_View_Initializing(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())
	// Don't set width - should show initializing.

	result := m.View()

	assert.Equal(t, "Initializing...", result)
}

func TestModel_HandleWindowSizeMsg(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())

	msg := tea.WindowSizeMsg{
		Width:  100,
		Height: 50,
	}

	m.handleWindowSizeMsg(msg)

	assert.Equal(t, 100, m.width)
	assert.Equal(t, 50, m.height)
}

func TestTickCmd(t *testing.T) {
	cmd := tickCmd()
	assert.NotNil(t, cmd)
}

func TestModel_Update_KeyMsg(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())
	m.initialSnap = perf.Snapshot{
		Rows: []perf.Row{
			{Name: "test.Function", Count: 1, Total: 10 * time.Millisecond},
		},
	}
	m.updatePerformanceData()

	tests := []struct {
		name         string
		key          string
		expectedMode string
		shouldQuit   bool
	}{
		{
			name:         "Switch to bar mode",
			key:          "1",
			expectedMode: "bar",
			shouldQuit:   false,
		},
		{
			name:         "Switch to sparkline mode",
			key:          "2",
			expectedMode: "sparkline",
			shouldQuit:   false,
		},
		{
			name:         "Switch to table mode",
			key:          "3",
			expectedMode: "table",
			shouldQuit:   false,
		},
		{
			name:         "Quit with q",
			key:          "q",
			expectedMode: "bar",
			shouldQuit:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.visualMode = "bar"
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}

			newModel, cmd := m.Update(msg)

			if tt.shouldQuit {
				assert.NotNil(t, cmd)
			} else {
				assert.Equal(t, tt.expectedMode, newModel.(*model).visualMode)
			}
		})
	}
}

func TestModel_Update_WindowSizeMsg(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())

	msg := tea.WindowSizeMsg{Width: 120, Height: 60}

	newModel, _ := m.Update(msg)

	assert.Equal(t, 120, newModel.(*model).width)
	assert.Equal(t, 60, newModel.(*model).height)
}

func TestModel_HandleKeyMsg_Navigation(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())
	m.initialSnap = perf.Snapshot{
		Rows: []perf.Row{
			{Name: "test.Function1", Count: 1, Total: 10 * time.Millisecond},
			{Name: "test.Function2", Count: 2, Total: 20 * time.Millisecond},
			{Name: "test.Function3", Count: 3, Total: 30 * time.Millisecond},
		},
	}
	m.updatePerformanceData()

	// Test down navigation.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	m.Update(msg)

	// Test up navigation.
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
	m.Update(msg)
}

func TestModel_RenderLegend(t *testing.T) {
	hm := NewHeatModel()
	m := newModel(hm, "bar", context.Background())

	tests := []struct {
		name            string
		snapshot        perf.Snapshot
		expectParallel  bool
		expectedStrings []string
	}{
		{
			name: "Single threaded execution",
			snapshot: perf.Snapshot{
				Rows: []perf.Row{
					{Name: "test.Function", Count: 1, Total: 50 * time.Millisecond},
				},
				Elapsed: 100 * time.Millisecond,
			},
			expectParallel: false,
			expectedStrings: []string{
				"Parallelism:",
				"Elapsed:",
				"CPU Time:",
				"Count: # calls",
				"CPU Time: sum of self-time",
			},
		},
		{
			name: "Parallel execution with multiple functions",
			snapshot: perf.Snapshot{
				Rows: []perf.Row{
					{Name: "test.Function1", Count: 10, Total: 100 * time.Millisecond},
					{Name: "test.Function2", Count: 20, Total: 200 * time.Millisecond},
					{Name: "test.Function3", Count: 30, Total: 300 * time.Millisecond},
				},
				Elapsed: 200 * time.Millisecond,
			},
			expectParallel: true,
			expectedStrings: []string{
				"Parallelism:",
				"Elapsed:",
				"CPU Time:",
				"Count: # calls",
				"CPU Time: sum of self-time",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.initialSnap = tt.snapshot

			result := m.renderLegend()

			// Verify all expected strings are present.
			for _, expected := range tt.expectedStrings {
				assert.Contains(t, result, expected)
			}

			// Verify parallelism is calculated (value greater than 0).
			assert.Contains(t, result, "~")
		})
	}
}
