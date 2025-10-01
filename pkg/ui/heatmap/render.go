package heatmap

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
)

func (m *model) renderVisualization() string {
	switch m.visualMode {
	case "bar":
		return m.renderBarChart()
	case "ascii":
		return m.renderASCIIHeatMap()
	case "table":
		return m.renderTableHeatMap()
	case "sparkline":
		return m.renderSparklines()
	default:
		return m.renderBarChart()
	}
}

func (m *model) renderBarChart() string {
	snap := perf.SnapshotTop("total", topFunctionsLimit)

	if len(snap.Rows) == 0 {
		return heatMapStyle.Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No performance data available"))
	}

	bars := m.renderBarsFromPerf(snap)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("🔥 Performance Heatmap - Bar Chart")

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title, ""}, bars...)...)
	return heatMapStyle.Render(content)
}

func (m *model) renderBarsFromPerf(snap perf.Snapshot) []string {
	var bars []string

	if len(snap.Rows) == 0 {
		return bars
	}

	// Find max total time for scaling.
	maxTotal := snap.Rows[0].Total
	for _, r := range snap.Rows {
		if r.Total > maxTotal {
			maxTotal = r.Total
		}
	}

	if maxTotal == 0 {
		return bars
	}

	barColors := []lipgloss.Color{
		lipgloss.Color("205"), // Pink
		lipgloss.Color("214"), // Orange
		lipgloss.Color("226"), // Yellow
		lipgloss.Color("118"), // Green
		lipgloss.Color("39"),  // Blue
		lipgloss.Color("170"), // Purple
	}

	for i, r := range snap.Rows {
		ratio := float64(r.Total) / float64(maxTotal)
		barWidth := int(ratio * float64(maxBarChartWidth))
		if barWidth < 1 && r.Total > 0 {
			barWidth = 1
		}

		bar := strings.Repeat("█", barWidth)
		coloredBar := lipgloss.NewStyle().
			Foreground(barColors[i%len(barColors)]).
			Render(bar)

		// Truncate function name if too long.
		funcName := r.Name
		if len(funcName) > funcNameBarWidth {
			funcName = funcName[:funcNameBarTruncate] + "..."
		}

		label := lipgloss.NewStyle().
			Width(funcNameBarWidth).
			Align(lipgloss.Left).
			Render(funcName)

		value := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf(horizontalSpace+"%s", formatDuration(r.Total)))

		bars = append(bars, lipgloss.JoinHorizontal(lipgloss.Left, label, horizontalSpace, coloredBar, value))
	}

	return bars
}

func (m *model) renderASCIIHeatMap() string {
	snap := perf.SnapshotTop("total", topFunctionsLimit)

	if len(snap.Rows) == 0 {
		return heatMapStyle.Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No performance data available"))
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("🔥 Performance Heatmap - Function Times")

	rows := m.renderASCIIBarsFromPerf(snap)

	lines := append([]string{title, ""}, rows...)

	content := strings.Join(lines, "\n")
	return heatMapStyle.Render(content)
}

func (m *model) renderASCIIBarsFromPerf(snap perf.Snapshot) []string {
	var rows []string

	if len(snap.Rows) == 0 {
		return rows
	}

	maxTotal := m.findMaxTotal(snap.Rows)
	if maxTotal == 0 {
		return rows
	}

	for _, r := range snap.Rows {
		row := m.renderASCIIBarRow(r, maxTotal)
		rows = append(rows, row)
	}

	return rows
}

func (m *model) findMaxTotal(rows []perf.Row) time.Duration {
	maxTotal := rows[0].Total
	for _, r := range rows {
		if r.Total > maxTotal {
			maxTotal = r.Total
		}
	}
	return maxTotal
}

func (m *model) renderASCIIBarRow(r perf.Row, maxTotal time.Duration) string {
	// Create visual bar based on total time.
	ratio := float64(r.Total) / float64(maxTotal)
	barWidth := int(ratio * 40)
	if barWidth < 1 && r.Total > 0 {
		barWidth = 1
	}

	bar := strings.Repeat("█", barWidth)
	color := m.selectColorByDuration(r.Total)
	coloredBar := lipgloss.NewStyle().Foreground(color).Render(bar)

	// Truncate function name if too long.
	funcName := r.Name
	if len(funcName) > funcNameMaxWidth {
		funcName = funcName[:funcNameTruncateWidth] + "..."
	}

	label := lipgloss.NewStyle().
		Width(funcNameMaxWidth).
		Align(lipgloss.Left).
		Render(funcName)

	value := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf(horizontalSpace+"%s", formatDuration(r.Total)))

	return lipgloss.JoinHorizontal(lipgloss.Left, label, horizontalSpace, coloredBar, value)
}

func (m *model) selectColorByDuration(d time.Duration) lipgloss.Color {
	us := d.Microseconds()
	switch {
	case us > thresholdRed: // >1ms
		return lipgloss.Color("196") // Red
	case us > thresholdYellow: // >500µs
		return lipgloss.Color("214") // Orange
	case us > thresholdGreen: // >100µs
		return lipgloss.Color("226") // Yellow
	default:
		return lipgloss.Color("118") // Green
	}
}

func (m *model) renderTableHeatMap() string {
	// Return the actual performance table view instead of mock data.
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("🔥 Performance Heatmap - Table View")

	// Use the table that's already configured with real perf data.
	tableView := m.table.View()

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", tableView)
	return heatMapStyle.Render(content)
}

func (m *model) renderSparklines() string {
	snap := perf.SnapshotTop("total", topFunctionsLimit)

	if len(snap.Rows) == 0 {
		return heatMapStyle.Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render("No performance data available"))
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("🔥 Performance Heatmap - Sparklines")

	sparklines := m.renderSparklinesFromPerf(snap)

	lines := append([]string{title, ""}, sparklines...)
	content := strings.Join(lines, "\n")
	return heatMapStyle.Render(content)
}

func (m *model) renderSparklinesFromPerf(snap perf.Snapshot) []string {
	var lines []string

	if len(snap.Rows) == 0 {
		return lines
	}

	// Sparkline characters from low to high.
	sparkChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Find max avg for normalization.
	maxAvg := snap.Rows[0].Avg
	for _, r := range snap.Rows {
		if r.Avg > maxAvg {
			maxAvg = r.Avg
		}
	}

	if maxAvg == 0 {
		return lines
	}

	for _, r := range snap.Rows {
		// Create a simple visualization showing min/avg/max relationship.
		// Since we don't have historical data, we'll show the distribution.
		ratio := float64(r.Avg) / float64(maxAvg)
		sparkIndex := int(ratio * float64(len(sparkChars)-1))
		if sparkIndex >= len(sparkChars) {
			sparkIndex = len(sparkChars) - 1
		}

		// Create a simple bar showing relative average.
		sparkLine := strings.Repeat(string(sparkChars[sparkIndex]), sparklineRepeat)

		// Truncate function name if too long.
		funcName := r.Name
		if len(funcName) > funcNameBarWidth {
			funcName = funcName[:funcNameBarTruncate] + "..."
		}

		label := lipgloss.NewStyle().
			Width(funcNameBarWidth).
			Align(lipgloss.Left).
			Render(funcName)

		spark := lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Render(sparkLine)

		stats := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf(horizontalSpace+"%s (×%d)", formatDuration(r.Avg), r.Count))

		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, label, horizontalSpace, spark, stats))
	}

	return lines
}
