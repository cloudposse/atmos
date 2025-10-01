package heatmap

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Table dimensions.
	tableFunctionWidth = 50
	tableCountWidth    = 8
	tableDurationWidth = 10
	tableHeight        = 10

	// Display limits.
	maxHeatMapDisplayColumns = 20
	maxTableDisplayRuns      = 8
	maxSparklineWidth        = 50
	maxBarChartWidth         = 40
	topFunctionsLimit        = 15

	// Performance thresholds (microseconds).
	thresholdGreen  = 100
	thresholdYellow = 500
	thresholdRed    = 1000

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

type Step string

const (
	StepParseConfig Step = "parse_config"
	StepLoadStacks  Step = "load_stacks"
	StepResolveDeps Step = "resolve_deps"
	StepPlan        Step = "terraform_plan"
	StepApply       Step = "terraform_apply"
)

var Steps = []Step{StepParseConfig, StepLoadStacks, StepResolveDeps, StepPlan, StepApply}

type RunSample struct {
	RunIndex int
	StepDur  map[Step]time.Duration
}

type HeatModel struct {
	mu        sync.Mutex
	runCount  int
	stepIndex map[Step]int
	matrix    [][]float64 // [row=step][col=run] in ms
}

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
			if maxV == minV {
				norm[r][c] = 0
			} else {
				norm[r][c] = (v - minV) / (maxV - minV)
			}
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
	initialSnap perf.Snapshot // Snapshot captured at TUI start
}

// Messages.
type tickMsg time.Time

// Styles.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	heatMapStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD")).
			Padding(1, 2)

	tableStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#874BFD"))
)

func newModel(heatModel *HeatModel, mode string, ctx context.Context) *model {
	// Initialize table.
	columns := []table.Column{
		{Title: "Function", Width: tableFunctionWidth},
		{Title: "Count", Width: tableCountWidth},
		{Title: "Total", Width: tableDurationWidth},
		{Title: "Avg", Width: tableDurationWidth},
		{Title: "Max", Width: tableDurationWidth},
		{Title: "P95", Width: tableCountWidth},
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

func (m *model) Init() tea.Cmd {
	// Capture snapshot at TUI start to freeze elapsed time.
	m.initialSnap = perf.SnapshotTop("total", topFunctionsLimit)

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
	case "1", "2", "3", "4":
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
		"2": "ascii",
		"3": "table",
		"4": "sparkline",
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
	// Update table with latest performance data.
	snap := perf.SnapshotTop("total", topFunctionsLimit)
	rows := []table.Row{}

	for _, r := range snap.Rows {
		p95 := "-"
		if r.P95 > 0 {
			p95 = formatDuration(r.P95)
		}
		rows = append(rows, table.Row{
			truncate(r.Name, tableFunctionWidth-2),
			fmt.Sprintf("%d", r.Count),
			formatDuration(r.Total),
			formatDuration(r.Avg),
			formatDuration(r.Max),
			p95,
		})
	}
	m.table.SetRows(rows)
}

func (m *model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	var sections []string

	// Header.
	header := headerStyle.Width(m.width - 2).Render(
		fmt.Sprintf("Atmos Performance Results - %s Mode (Press 1-4 to switch modes, q/esc to quit)",
			toTitle(m.visualMode)))
	sections = append(sections, header)

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

// formatDuration formats a duration for display, showing "0" instead of "0s" for zero durations.
func formatDuration(d time.Duration) string {
	truncated := d.Truncate(time.Microsecond)
	if truncated == 0 {
		return "0"
	}
	return truncated.String()
}

// StartBubbleTeaUI starts the Bubble Tea interface.
func StartBubbleTeaUI(ctx context.Context, heatModel *HeatModel, mode string) error {
	m := newModel(heatModel, mode, ctx)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Run the program directly (blocking).
	_, err := p.Run()
	return err
}
