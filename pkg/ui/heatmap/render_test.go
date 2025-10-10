package heatmap

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/perf"
)

func TestModel_RenderVisualization(t *testing.T) {
	tests := []struct {
		name       string
		visualMode string
		expectFunc func(string) bool
	}{
		{
			name:       "Bar mode",
			visualMode: "bar",
			expectFunc: func(s string) bool {
				return len(s) > 0
			},
		},
		{
			name:       "Sparkline mode",
			visualMode: "sparkline",
			expectFunc: func(s string) bool {
				return len(s) > 0
			},
		},
		{
			name:       "Table mode",
			visualMode: "table",
			expectFunc: func(s string) bool {
				return len(s) > 0
			},
		},
		{
			name:       "Default (unknown mode defaults to bar)",
			visualMode: "unknown",
			expectFunc: func(s string) bool {
				return len(s) > 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model{
				visualMode: tt.visualMode,
				heatModel:  NewHeatModel(),
				table:      table.New(),
				initialSnap: perf.Snapshot{
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
				},
			}

			result := m.renderVisualization()
			assert.True(t, tt.expectFunc(result))
		})
	}
}

func TestModel_RenderBarChart(t *testing.T) {
	m := &model{
		heatModel: NewHeatModel(),
		initialSnap: perf.Snapshot{
			Rows: []perf.Row{
				{
					Name:  "pkg.Function1",
					Count: 10,
					Total: 100 * time.Millisecond,
					Avg:   10 * time.Millisecond,
					Max:   20 * time.Millisecond,
				},
				{
					Name:  "pkg.Function2",
					Count: 5,
					Total: 50 * time.Millisecond,
					Avg:   10 * time.Millisecond,
					Max:   15 * time.Millisecond,
				},
			},
		},
	}

	result := m.renderBarChart()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Performance Heatmap - Bar Chart")
}

func TestModel_RenderBarChart_NoData(t *testing.T) {
	m := &model{
		heatModel: NewHeatModel(),
		initialSnap: perf.Snapshot{
			Rows: []perf.Row{},
		},
	}

	result := m.renderBarChart()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "No performance data available")
}

func TestModel_RenderBarsFromPerf(t *testing.T) {
	m := &model{}

	snap := perf.Snapshot{
		Rows: []perf.Row{
			{
				Name:  "test.Function1",
				Count: 10,
				Total: 200 * time.Millisecond,
			},
			{
				Name:  "test.Function2",
				Count: 5,
				Total: 100 * time.Millisecond,
			},
		},
	}

	bars := m.renderBarsFromPerf(snap)

	assert.Len(t, bars, 2)
	// First bar should be for the function with the longest total time.
	assert.Contains(t, bars[0], "Function1")
	assert.Contains(t, bars[1], "Function2")
}

func TestModel_RenderBarsFromPerf_ZeroTotal(t *testing.T) {
	m := &model{}

	snap := perf.Snapshot{
		Rows: []perf.Row{
			{
				Name:  "test.Function",
				Count: 10,
				Total: 0, // Zero total
			},
		},
	}

	bars := m.renderBarsFromPerf(snap)

	// Should return empty bars for zero total.
	assert.Empty(t, bars)
}

func TestModel_RenderSparklines(t *testing.T) {
	m := &model{
		heatModel: NewHeatModel(),
		initialSnap: perf.Snapshot{
			Rows: []perf.Row{
				{
					Name:  "pkg.Function",
					Count: 10,
					Total: 100 * time.Millisecond,
					Avg:   10 * time.Millisecond,
				},
			},
		},
	}

	result := m.renderSparklines()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Performance Heatmap - Sparklines")
}

func TestModel_RenderSparklines_NoData(t *testing.T) {
	m := &model{
		heatModel: NewHeatModel(),
		initialSnap: perf.Snapshot{
			Rows: []perf.Row{},
		},
	}

	result := m.renderSparklines()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "No performance data available")
}

func TestModel_RenderSparklinesFromPerf(t *testing.T) {
	m := &model{}

	snap := perf.Snapshot{
		Rows: []perf.Row{
			{
				Name:  "test.Function",
				Count: 10,
				Total: 100 * time.Millisecond,
				Avg:   10 * time.Millisecond,
			},
		},
	}

	lines := m.renderSparklinesFromPerf(snap)

	assert.Len(t, lines, 1)
	assert.Contains(t, lines[0], "Function")
}

func TestModel_RenderTableHeatMap(t *testing.T) {
	m := &model{
		heatModel: NewHeatModel(),
		table:     table.New(),
	}

	result := m.renderTableHeatMap()

	assert.NotEmpty(t, result)
	assert.Contains(t, result, "Performance Heatmap - Table View")
}

func TestNewModel(t *testing.T) {
	hm := NewHeatModel()
	ctx := context.Background()
	mode := "bar"

	m := newModel(hm, mode, ctx)

	assert.NotNil(t, m)
	assert.Equal(t, hm, m.heatModel)
	assert.Equal(t, mode, m.visualMode)
	assert.Equal(t, ctx, m.ctx)
	assert.NotNil(t, m.table)
}

func TestModel_HandleVisualizationModeKey(t *testing.T) {
	m := &model{
		visualMode: "bar",
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"1", "bar"},
		{"2", "sparkline"},
		{"3", "table"},
		{"4", "bar"}, // Invalid key, should not change mode
	}

	for _, tt := range tests {
		t.Run("Key_"+tt.key, func(t *testing.T) {
			m.visualMode = "bar"
			m.handleVisualizationModeKey(tt.key)
			assert.Equal(t, tt.expected, m.visualMode)
		})
	}
}
