package heatmap

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

func (m *model) renderVisualization() string {
	switch m.visualMode {
	case "bar":
		return m.renderBarChart()
	case "table":
		return m.renderTableHeatMap()
	case "sparkline":
		return m.renderSparklines()
	default:
		return m.renderBarChart()
	}
}

// renderBarChart displays the highest-cost functions as bars using the frozen performance snapshot.
func (m *model) renderBarChart() string {
	// Use the frozen snapshot captured at TUI start, limited to top functions for visual display.
	snap := m.getLimitedSnapshot()

	if len(snap.Rows) == 0 {
		return theme.GetCurrentStyles().Border.Padding(1, 2).Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)).
			Render("No performance data available"))
	}

	bars := m.renderBarsFromPerf(snap)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Primary)).
		Render("🔥 Performance Heatmap - Bar Chart")

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title, ""}, bars...)...)
	return theme.GetCurrentStyles().Border.Padding(1, 2).Render(content)
}

// getBarColorGradient returns a color gradient from red (slowest/top) to green (fastest/bottom).
func getBarColorGradient() []lipgloss.Color {
	return []lipgloss.Color{
		lipgloss.Color("196"), // Red
		lipgloss.Color("202"), // Orange-Red
		lipgloss.Color("208"), // Orange
		lipgloss.Color("214"), // Yellow-Orange
		lipgloss.Color("226"), // Yellow
		lipgloss.Color("190"), // Yellow-Green
		lipgloss.Color("154"), // Light Green
		lipgloss.Color("118"), // Green
	}
}

// getColorForPosition returns the appropriate color for a bar based on its position in the list.
func getColorForPosition(position, totalItems int, colors []lipgloss.Color) lipgloss.Color {
	colorIndex := int(float64(position) / float64(totalItems) * float64(len(colors)))
	if colorIndex >= len(colors) {
		colorIndex = len(colors) - 1
	}
	return colors[colorIndex]
}

// renderBarsFromPerf scales function bars against the largest total duration in the snapshot.
func (m *model) renderBarsFromPerf(snap perf.Snapshot) []string {
	var bars []string

	if len(snap.Rows) == 0 {
		return bars
	}

	maxTotal := m.findMaxTotal(snap.Rows)
	if maxTotal == 0 {
		return bars
	}

	barColors := getBarColorGradient()

	for i, r := range snap.Rows {
		ratio := float64(r.Total) / float64(maxTotal)
		barWidth := int(ratio * float64(maxBarChartWidth))
		if barWidth < 1 && r.Total > 0 {
			barWidth = 1
		}

		color := getColorForPosition(i, len(snap.Rows), barColors)
		bar := strings.Repeat("█", barWidth)
		coloredBar := lipgloss.NewStyle().
			Foreground(color).
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

		// Show average per call (more intuitive) with call count.
		value := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)).
			Render(fmt.Sprintf(horizontalSpace+"avg: %s | calls: %d", FormatDuration(r.Avg), r.Count))

		bars = append(bars, lipgloss.JoinHorizontal(lipgloss.Left, label, horizontalSpace, coloredBar, value))
	}

	return bars
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

// renderTableHeatMap wraps the performance table in the current theme's title and border styles.
func (m *model) renderTableHeatMap() string {
	// Return the actual performance table view instead of mock data.
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Primary)).
		Render("🔥 Performance Heatmap - Table View")

	// Use the table that's already configured with real perf data.
	tableView := m.table.View()

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", tableView)
	return theme.GetCurrentStyles().Border.Padding(1, 2).Render(content)
}

// renderSparklines displays compact function timings from the frozen performance snapshot.
func (m *model) renderSparklines() string {
	// Use the frozen snapshot captured at TUI start, limited to top functions for visual display.
	snap := m.getLimitedSnapshot()

	if len(snap.Rows) == 0 {
		return theme.GetCurrentStyles().Border.Padding(1, 2).Render(lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)).
			Render("No performance data available"))
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Primary)).
		Render("🔥 Performance Heatmap - Sparklines")

	sparklines := m.renderSparklinesFromPerf(snap)

	lines := append([]string{title, ""}, sparklines...)
	content := strings.Join(lines, "\n")
	return theme.GetCurrentStyles().Border.Padding(1, 2).Render(content)
}

// renderSparklinesFromPerf scales function sparklines against the largest average duration in the snapshot.
func (m *model) renderSparklinesFromPerf(snap perf.Snapshot) []string {
	var lines []string

	if len(snap.Rows) == 0 {
		return lines
	}

	// Sparkline characters from low to high.
	sparkChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Find max avg self-time for normalization.
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
			Foreground(lipgloss.Color(theme.GetCurrentColorScheme().Link)).
			Render(sparkLine)

		// Show average per call with call count for consistency with bar chart.
		stats := lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.GetCurrentColorScheme().TextMuted)).
			Render(fmt.Sprintf(horizontalSpace+"avg: %s | calls: %d", FormatDuration(r.Avg), r.Count))

		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, label, horizontalSpace, spark, stats))
	}

	return lines
}
