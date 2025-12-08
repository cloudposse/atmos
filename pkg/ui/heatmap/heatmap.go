package heatmap

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// Table dimensions.
	tableFunctionWidth = 50
	tableCountWidth    = 8
	tableDurationWidth = 13 // Wider to accommodate durations like "202.367ms" without truncation
	tableHeight        = 20

	// Display limits.
	maxHeatMapDisplayColumns = 20
	maxTableDisplayRuns      = 8
	maxSparklineWidth        = 50
	maxBarChartWidth         = 40
	topFunctionsLimit        = 50 // For the full table view
	topFunctionsVisualLimit  = 25 // For bar chart, sparkline, and table views

	// Function name truncation.
	funcNameMaxWidth      = 50
	funcNameTruncateWidth = 47
	funcNameBarWidth      = 50
	funcNameBarTruncate   = 47
	sparklineRepeat       = 10

	// Formatting.
	stepLabelWidth  = 15
	tickInterval    = 200 * time.Millisecond
	leftAlignFormat = "%-*s"
	horizontalSpace = " "
)

// Step represents a stage in the Atmos workflow execution lifecycle.
type Step string

const (
	StepParseConfig Step = "parse_config"
	StepLoadStacks  Step = "load_stacks"
	StepResolveDeps Step = "resolve_deps"
	StepPlan        Step = "terraform_plan"
	StepApply       Step = "terraform_apply"
)

// Steps is the ordered list of all workflow execution steps.
var Steps = []Step{StepParseConfig, StepLoadStacks, StepResolveDeps, StepPlan, StepApply}

// RunSample represents a single execution run with timing data for each step.
type RunSample struct {
	RunIndex int
	StepDur  map[Step]time.Duration
}

// HeatModel stores performance timing data for multiple execution runs in a matrix format.
type HeatModel struct {
	mu        sync.Mutex
	runCount  int
	stepIndex map[Step]int
	matrix    [][]float64 // [row=step][col=run] in ms
}

// NewHeatModel creates and initializes a new HeatModel with empty timing matrix.
func NewHeatModel() *HeatModel {
	sm := &HeatModel{stepIndex: make(map[Step]int)}
	for i, s := range Steps {
		sm.stepIndex[s] = i
	}
	sm.matrix = make([][]float64, len(Steps))
	for i := range sm.matrix {
		sm.matrix[i] = make([]float64, 0, perf.DefaultMatrixCapacity)
	}
	return sm
}

// AddRun adds timing data from a single execution run to the heatmap model.
func (m *HeatModel) AddRun(sample RunSample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.matrix {
		if len(m.matrix[i]) < sample.RunIndex+1 {
			m.matrix[i] = append(m.matrix[i], make([]float64, sample.RunIndex+1-len(m.matrix[i]))...)
		}
	}
	for s, d := range sample.StepDur {
		row := m.stepIndex[s]
		m.matrix[row][sample.RunIndex] = float64(d.Milliseconds())
	}
	if sample.RunIndex+1 > m.runCount {
		m.runCount = sample.RunIndex + 1
	}
}

// Normalized returns a normalized copy of the timing matrix with values scaled to [0,1] range, along with min and max values in milliseconds.
func (m *HeatModel) Normalized() ([][]float64, float64, float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	minV, maxV := m.findMinMax()
	norm := m.normalizeMatrix(minV, maxV)

	return norm, minV, maxV
}

func (m *HeatModel) findMinMax() (float64, float64) {
	minV := math.MaxFloat64
	maxV := -1.0

	for r := range m.matrix {
		for c := range m.matrix[r] {
			v := m.matrix[r][c]
			if v <= 0 {
				continue
			}
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}

	if maxV < 0 || minV == math.MaxFloat64 {
		return 0, 0
	}

	return minV, maxV
}

func (m *HeatModel) normalizeMatrix(minV, maxV float64) [][]float64 {
	norm := make([][]float64, len(m.matrix))
	for r := range m.matrix {
		norm[r] = make([]float64, len(m.matrix[r]))
		for c := range m.matrix[r] {
			v := m.matrix[r][c]
			if v <= 0 {
				continue
			}
			if maxV == minV {
				norm[r][c] = 1
				continue
			}
			n := (v - minV) / (maxV - minV)
			if n < 0 {
				n = 0
			}
			norm[r][c] = n
		}
	}
	return norm
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// toTitle converts the first character of a string to uppercase.
func toTitle(s string) string {
	if len(s) == 0 {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// BubbleTea Model.
type model struct {
	heatModel   *HeatModel
	visualMode  string
	table       table.Model
	width       int
	height      int
	lastUpdate  time.Time
	ctx         context.Context
	initialSnap perf.Snapshot // Snapshot captured at TUI start (filtered)
}

// Messages.
type tickMsg time.Time

// Styles.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(theme.ColorWhite)).
			Background(lipgloss.Color(theme.ColorBlue)).
			Padding(0, 1)

	heatMapStyle = theme.Styles.Border.
			Padding(1, 2)

	tableStyle = theme.Styles.Border
)

func newModel(heatModel *HeatModel, mode string, ctx context.Context) *model {
	// Initialize table.
	columns := []table.Column{
		{Title: "Function", Width: tableFunctionWidth},
		{Title: "Count", Width: tableCountWidth},
		{Title: "CPU Time", Width: tableDurationWidth},
		{Title: "Avg", Width: tableDurationWidth},
		{Title: "Max", Width: tableDurationWidth},
		{Title: "P95", Width: tableDurationWidth},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(tableHeight),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(s)

	return &model{
		heatModel:  heatModel,
		visualMode: mode,
		table:      t,
		ctx:        ctx,
		lastUpdate: time.Now(),
	}
}

// getLimitedSnapshot returns a copy of the initial snapshot with only the top N functions for visual displays.
func (m *model) getLimitedSnapshot() perf.Snapshot {
	snap := m.initialSnap
	if len(snap.Rows) > topFunctionsVisualLimit {
		snap.Rows = snap.Rows[:topFunctionsVisualLimit]
	}
	return snap
}

func (m *model) Init() tea.Cmd {
	// Capture unbounded snapshot at TUI start to freeze elapsed time and include all functions.
	// Filter out functions with zero total time for cleaner display.
	// topN=0 means no limit, capturing all tracked functions for accurate CPU time calculation.
	m.initialSnap = perf.SnapshotTopFiltered("total", 0)

	// Load initial performance data.
	m.updatePerformanceData()

	return tea.Batch(
		tickCmd(),
		tea.EnterAltScreen,
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
	case tickMsg:
		if cmd := m.handleTickMsg(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update table for navigation.
	m.table, cmd = m.table.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "1", "2", "3":
		m.handleVisualizationModeKey(msg.String())
		return m, nil
	case "up", "k":
		return m, m.handleNavigationUp(msg)
	case "down", "j":
		return m, m.handleNavigationDown(msg)
	default:
		return m, m.handleDefaultKey(msg)
	}
}

func (m *model) handleVisualizationModeKey(key string) {
	modes := map[string]string{
		"1": "bar",
		"2": "sparkline",
		"3": "table",
	}
	if mode, ok := modes[key]; ok {
		m.visualMode = mode
	}
}

func (m *model) handleNavigationUp(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	// Wraparound: if at top, go to bottom.
	if m.table.Cursor() == 0 {
		m.table.SetCursor(len(m.table.Rows()) - 1)
	} else {
		m.table, cmd = m.table.Update(msg)
	}
	return cmd
}

func (m *model) handleNavigationDown(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	// Wraparound: if at bottom, go to top.
	if m.table.Cursor() == len(m.table.Rows())-1 {
		m.table.SetCursor(0)
	} else {
		m.table, cmd = m.table.Update(msg)
	}
	return cmd
}

func (m *model) handleDefaultKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	// Pass other keys to the table.
	m.table, cmd = m.table.Update(msg)
	return cmd
}

func (m *model) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	// Don't set table width - let it use natural column widths for consistency with visualization.
}

func (m *model) handleTickMsg(msg tickMsg) tea.Cmd {
	// Check if context is done.
	select {
	case <-m.ctx.Done():
		return tea.Quit
	default:
	}

	// For static data display, we only update the table once and don't need continuous updates.
	if m.lastUpdate.IsZero() {
		m.updatePerformanceData()
		m.lastUpdate = time.Time(msg)
	}

	// Continue ticking for responsiveness but don't update data.
	return tickCmd()
}

func (m *model) updatePerformanceData() {
	// Update table with frozen snapshot from TUI start.
	// The snapshot is already filtered to exclude zero-time functions.
	// Limit table display to top N functions for readability.
	rows := []table.Row{}

	displayRows := m.initialSnap.Rows
	if len(displayRows) > topFunctionsLimit {
		displayRows = displayRows[:topFunctionsLimit]
	}

	for _, r := range displayRows {
		p95 := "-"
		if r.P95 > 0 {
			p95 = FormatDuration(r.P95)
		}
		rows = append(rows, table.Row{
			truncate(r.Name, tableFunctionWidth-2),
			fmt.Sprintf("%d", r.Count),
			FormatDuration(r.Total),
			FormatDuration(r.Avg),
			FormatDuration(r.Max),
			p95,
		})
	}
	m.table.SetRows(rows)
}

func (m *model) renderLegend() string {
	legendStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Padding(0, 2)

	// Calculate total CPU time and parallelism from frozen snapshot (all tracked functions).
	// Using m.initialSnap ensures elapsed time doesn't keep increasing in the TUI.
	var totalCPUTime time.Duration
	for _, r := range m.initialSnap.Rows {
		totalCPUTime += r.Total
	}
	elapsed := m.initialSnap.Elapsed
	var parallelism float64
	if elapsed > 0 {
		parallelism = float64(totalCPUTime) / float64(elapsed)
	} else {
		parallelism = 0
	}

	legend := legendStyle.Render(
		fmt.Sprintf("Parallelism: ~%.1fx | Elapsed: %s | CPU Time: %s\n",
			parallelism,
			elapsed.Truncate(time.Microsecond),
			totalCPUTime.Truncate(time.Microsecond)) +
			"Count: # calls (incl. recursion) | CPU Time: sum of self-time (excludes children)\n" +
			"Avg: avg self-time | Max: max self-time | P95: 95th percentile self-time")

	return legend
}

func (m *model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	// Header.
	header := headerStyle.Width(m.width - 2).Render(
		fmt.Sprintf("Atmos Performance Results - %s Mode (Press 1-3 to switch modes, q/esc to quit)",
			toTitle(m.visualMode)))
	sections = append(sections, header)

	// Legend.
	legend := m.renderLegend()
	sections = append(sections, legend)

	// Visualization section.
	visualization := m.renderVisualization()
	sections = append(sections, visualization)

	// Performance table.
	tableSection := tableStyle.Render(m.table.View())
	sections = append(sections, tableSection)

	// Status bar with frozen performance data from TUI start.
	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf("📊 Command completed | Functions: %d | Total Calls: %d | Elapsed: %s | Press q/esc to quit",
			m.initialSnap.TotalFuncs, m.initialSnap.TotalCalls, m.initialSnap.Elapsed.Truncate(time.Microsecond)))
	sections = append(sections, status)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// FormatDuration formats a duration for display, showing "0" instead of "0s" for zero durations.
func FormatDuration(d time.Duration) string {
	truncated := d.Truncate(time.Microsecond)
	if truncated == 0 {
		return "0"
	}
	return truncated.String()
}

// StartBubbleTeaUI starts the Bubble Tea interface.
func StartBubbleTeaUI(ctx context.Context, heatModel *HeatModel, mode string) error {
	m := newModel(heatModel, mode, ctx)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
		tea.WithContext(ctx)) // Pass context for proper cancellation

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("%w: failed to run performance heatmap TUI: %v", errUtils.ErrTUIRun, err)
	}
	return nil
}
