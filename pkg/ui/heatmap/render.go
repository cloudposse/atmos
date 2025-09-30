package heatmap

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	raw := m.copyMatrixData()
	averages, maxValue := m.calculateAverages(raw)
	bars := m.renderBars(averages, maxValue)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("Average Performance by Step")

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{title, ""}, bars...)...)
	return heatMapStyle.Render(content)
}

func (m *model) copyMatrixData() [][]float64 {
	m.heatModel.mu.Lock()
	defer m.heatModel.mu.Unlock()

	raw := make([][]float64, len(m.heatModel.matrix))
	for i := range m.heatModel.matrix {
		raw[i] = append([]float64(nil), m.heatModel.matrix[i]...)
	}
	return raw
}

func (m *model) calculateAverages(raw [][]float64) ([]float64, float64) {
	averages := make([]float64, len(Steps))
	maxValue := 0.0

	for i := range averages {
		if len(raw[i]) == 0 {
			continue
		}

		var sum float64
		count := 0
		for _, v := range raw[i] {
			if v > 0 {
				sum += v
				count++
			}
		}

		if count > 0 {
			averages[i] = sum / float64(count)
			if averages[i] > maxValue {
				maxValue = averages[i]
			}
		}
	}

	return averages, maxValue
}

func (m *model) renderBars(averages []float64, maxValue float64) []string {
	var bars []string

	for i, s := range Steps {
		if maxValue == 0 {
			continue
		}

		ratio := averages[i] / maxValue
		barWidth := int(ratio * maxBarChartWidth)
		if barWidth < 1 && averages[i] > 0 {
			barWidth = 1
		}

		bar := strings.Repeat("█", barWidth)
		coloredBar := lipgloss.NewStyle().
			Foreground(barColors[i%len(barColors)]).
			Render(bar)

		label := lipgloss.NewStyle().
			Width(stepLabelWidth).
			Align(lipgloss.Right).
			Render(string(s))

		value := lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render(fmt.Sprintf(" %.0fms", averages[i]))

		bars = append(bars, lipgloss.JoinHorizontal(lipgloss.Left, label, " ", coloredBar, value))
	}

	return bars
}

func (m *model) renderASCIIHeatMap() string {
	norm, _, _ := m.heatModel.Normalized()
	if len(norm) == 0 {
		return heatMapStyle.Render("No data available")
	}

	title := m.renderHeatMapTitle("Performance Heat Map (Intensity)")
	maxCols := m.getMaxDisplayColumns(norm)
	header := m.renderHeatMapHeader(maxCols)
	rows := m.renderHeatMapRows(norm, maxCols)

	lines := append([]string{title, ""}, header)
	lines = append(lines, rows...)

	content := strings.Join(lines, "\n")
	return heatMapStyle.Render(content)
}

func (m *model) renderHeatMapTitle(title string) string {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render(title)
}

func (m *model) getMaxDisplayColumns(norm [][]float64) int {
	maxCols := 0
	for i := range norm {
		if len(norm[i]) > maxCols {
			maxCols = len(norm[i])
		}
	}
	if maxCols > maxHeatMapDisplayColumns {
		maxCols = maxHeatMapDisplayColumns
	}
	return maxCols
}

func (m *model) renderHeatMapHeader(maxCols int) string {
	header := fmt.Sprintf(leftAlignFormat, stepLabelWidth, "Step")
	for j := 0; j < maxCols; j++ {
		header += fmt.Sprintf("%3d", j)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(header)
}

func (m *model) renderHeatMapRows(norm [][]float64, maxCols int) []string {
	heatChars := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var lines []string

	for i, stepLabel := range Steps {
		if i >= len(norm) {
			break
		}

		line := fmt.Sprintf(leftAlignFormat, stepLabelWidth, string(stepLabel))
		line += m.renderHeatMapCells(norm[i], maxCols, heatChars)
		lines = append(lines, line)
	}

	return lines
}

func (m *model) renderHeatMapCells(row []float64, maxCols int, heatChars []rune) string {
	var cells string

	for j := 0; j < maxCols; j++ {
		if j < len(row) && row[j] > 0 {
			cells += m.renderHeatCell(row[j], heatChars)
		} else {
			cells += "   "
		}
	}

	return cells
}

func (m *model) renderHeatCell(value float64, heatChars []rune) string {
	intensity := int(value * float64(len(heatChars)-1))
	if intensity >= len(heatChars) {
		intensity = len(heatChars) - 1
	}

	char := string(heatChars[intensity])
	if intensity > 0 {
		colorIndex := intensity * len(heatColors) / len(heatChars)
		if colorIndex >= len(heatColors) {
			colorIndex = len(heatColors) - 1
		}
		char = lipgloss.NewStyle().
			Foreground(heatColors[colorIndex]).
			Render(char)
	}

	return fmt.Sprintf("%2s ", char)
}

func (m *model) renderTableHeatMap() string {
	raw, runCount := m.getTableData()
	title := m.renderHeatMapTitle("Performance Heat Map (ms per run)")

	if runCount > maxTableDisplayRuns {
		runCount = maxTableDisplayRuns
	}

	header := m.renderTableHeader(runCount)
	rows := m.renderTableRows(raw, runCount)

	lines := append([]string{title, ""}, header)
	lines = append(lines, rows...)

	content := strings.Join(lines, "\n")
	return heatMapStyle.Render(content)
}

func (m *model) getTableData() ([][]float64, int) {
	m.heatModel.mu.Lock()
	defer m.heatModel.mu.Unlock()

	raw := make([][]float64, len(m.heatModel.matrix))
	for i := range m.heatModel.matrix {
		raw[i] = append([]float64(nil), m.heatModel.matrix[i]...)
	}
	return raw, m.heatModel.runCount
}

func (m *model) renderTableHeader(runCount int) string {
	header := fmt.Sprintf(leftAlignFormat, stepLabelWidth, "Step")
	for j := 0; j < runCount; j++ {
		header += fmt.Sprintf("%8s", fmt.Sprintf("Run%d", j))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(header)
}

func (m *model) renderTableRows(raw [][]float64, runCount int) []string {
	var lines []string

	for i, s := range Steps {
		if i >= len(raw) {
			break
		}

		line := fmt.Sprintf(leftAlignFormat, stepLabelWidth, string(s))
		line += m.renderTableCells(raw[i], runCount)
		lines = append(lines, line)
	}

	return lines
}

func (m *model) renderTableCells(row []float64, runCount int) string {
	var cells string

	for j := 0; j < runCount; j++ {
		if j < len(row) && row[j] > 0 {
			cells += m.coloredDurationCell(row[j])
		} else {
			cells += fmt.Sprintf("%8s", "-")
		}
	}

	return cells
}

func (m *model) coloredDurationCell(value float64) string {
	formatted := fmt.Sprintf("%.0f", value)

	var colored string
	switch {
	case value > thresholdOrange:
		colored = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(formatted)
	case value > thresholdYellow:
		colored = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(formatted)
	case value > thresholdGreen:
		colored = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(formatted)
	default:
		colored = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(formatted)
	}

	return fmt.Sprintf("%8s", colored)
}

func (m *model) renderSparklines() string {
	raw := m.copyMatrixData()
	title := m.renderHeatMapTitle("Performance Trends per Step")
	sparklines := m.renderSparklineRows(raw)

	lines := append([]string{title, ""}, sparklines...)
	content := strings.Join(lines, "\n")
	return heatMapStyle.Render(content)
}

func (m *model) renderSparklineRows(raw [][]float64) []string {
	sparkChars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	var lines []string

	for i, s := range Steps {
		if i >= len(raw) || len(raw[i]) == 0 {
			continue
		}

		validData, avg, maxVal := m.processSparklineData(raw[i])
		if len(validData) == 0 {
			continue
		}

		sparkline := m.buildSparkline(validData, maxVal, sparkChars)
		line := m.formatSparklineLine(s, sparkline, avg, i)
		lines = append(lines, line)
	}

	return lines
}

func (m *model) processSparklineData(row []float64) ([]float64, float64, float64) {
	var validData []float64
	var sum, maxVal float64

	for _, v := range row {
		if v > 0 {
			validData = append(validData, v)
			sum += v
			if v > maxVal {
				maxVal = v
			}
		}
	}

	var avg float64
	if len(validData) > 0 {
		avg = sum / float64(len(validData))
	}

	return validData, avg, maxVal
}

func (m *model) buildSparkline(validData []float64, maxVal float64, sparkChars []rune) string {
	if len(validData) <= 1 {
		return "▄"
	}

	var sparkline string
	for j, val := range validData {
		if j >= maxSparklineWidth {
			break
		}

		intensity := int((val / maxVal) * float64(len(sparkChars)-1))
		if intensity >= len(sparkChars) {
			intensity = len(sparkChars) - 1
		}
		sparkline += string(sparkChars[intensity])
	}

	return sparkline
}

func (m *model) formatSparklineLine(step Step, sparkline string, avg float64, colorIndex int) string {
	coloredSparkline := lipgloss.NewStyle().
		Foreground(barColors[colorIndex%len(barColors)]).
		Render(sparkline)

	label := lipgloss.NewStyle().
		Width(stepLabelWidth).
		Align(lipgloss.Right).
		Render(string(step))

	avgText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf(" (avg: %.0fms)", avg))

	return lipgloss.JoinHorizontal(lipgloss.Left, label, " ", coloredSparkline, avgText)
}
