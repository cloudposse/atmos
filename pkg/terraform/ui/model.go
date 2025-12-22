package ui

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	progressBarWidth = 40
	tickInterval     = 100 * time.Millisecond

	// minDisplayDuration ensures the progress UI is visible for at least this long.
	// This helps users see the progress even for fast operations.
	minDisplayDuration = 5 * time.Second

	// ANSI escape sequences for terminal control.
	clearToEOL = "\x1b[K" // Clear from cursor to end of line.
	cursorUp   = "\x1b[A" // Move cursor up one line.
)

// Model is the bubbletea model for streaming terraform output.
type Model struct {
	tracker   *ResourceTracker
	parser    *Parser
	reader    io.Reader
	spinner   spinner.Model
	progress  progress.Model
	width     int
	height    int
	err       error
	exitCode  int
	done      bool
	startTime time.Time
	component string // Component name for display.
	stack     string // Stack name for display.
	command   string // "plan", "apply", "init", "refresh".
}

// messageMsg wraps a parsed terraform message.
type messageMsg struct {
	result *ParseResult
}

// doneMsg signals completion.
type doneMsg struct {
	exitCode int
	err      error
}

// delayedQuitMsg signals it's time to quit after the minimum display duration.
type delayedQuitMsg struct{}

// tickMsg for periodic updates.
type tickMsg time.Time

// NewModel creates a new streaming model.
func NewModel(component, stack, command string, reader io.Reader) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)

	return &Model{
		tracker:   NewResourceTracker(),
		parser:    NewParser(reader),
		reader:    reader,
		spinner:   s,
		progress:  p,
		component: component,
		stack:     stack,
		command:   command,
		startTime: time.Now(),
	}
}

// Init initializes the model.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.listenForMessages(),
		m.tick(),
	)
}

// listenForMessages creates a command that listens for parsed messages.
func (m *Model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		result, err := m.parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return doneMsg{exitCode: 0, err: nil}
			}
			return doneMsg{exitCode: 1, err: err}
		}
		return messageMsg{result: result}
	}
}

// tick creates a periodic tick for updating elapsed time.
func (m *Model) tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Update handles messages.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = min(progressBarWidth, msg.Width-10)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.done = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case tickMsg:
		// Just trigger a re-render for elapsed time updates.
		return m, m.tick()

	case messageMsg:
		if msg.result.Err == nil && msg.result.Message != nil {
			m.tracker.HandleMessage(msg.result.Message)
		}
		return m, m.listenForMessages()

	case doneMsg:
		m.exitCode = msg.exitCode
		m.err = msg.err

		// Check if we need to wait for minimum display duration.
		elapsed := time.Since(m.startTime)
		if elapsed < minDisplayDuration {
			// Wait for remaining time before quitting.
			remaining := minDisplayDuration - elapsed
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return delayedQuitMsg{}
			})
		}

		m.done = true
		return m, tea.Quit

	case delayedQuitMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the UI.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) View() string {
	if m.done {
		// Use carriage return and clear to end of line to prevent artifacts.
		return "\r" + clearToEOL + m.finalView()
	}
	return m.progressView()
}

// progressView renders the in-progress state.
func (m Model) progressView() string {
	var b strings.Builder

	// Styles.
	stackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	componentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	// Build spinner + command + stack/component.
	spin := m.spinner.View() + " "
	commandInfo := fmt.Sprintf("%s %s/%s",
		m.command,
		stackStyle.Render(m.stack),
		componentStyle.Render(m.component),
	)

	// Add current activity (e.g., "Reading data.http.weather").
	activityInfo := ""
	if currentOp := m.tracker.GetCurrentActivity(); currentOp != nil {
		activityVerb := m.formatActivityVerb(currentOp)
		opElapsed := time.Since(currentOp.StartTime).Seconds()
		activityInfo = mutedStyle.Render(fmt.Sprintf(" %s %s (%.1fs)", activityVerb, currentOp.Address, opElapsed))
	}

	// Progress bar and count.
	total := m.tracker.GetTotalCount()
	completed := m.tracker.GetCompletedCount()

	var progressInfo string
	if total > 0 {
		percent := float64(completed) / float64(total)
		progressBar := m.progress.ViewAs(percent)
		progressInfo = fmt.Sprintf("%s %d/%d", progressBar, completed, total)
	} else {
		elapsed := time.Since(m.startTime).Seconds()
		progressInfo = fmt.Sprintf("%.1fs", elapsed)
	}

	// Calculate available width and build inline layout.
	// Layout: spinner + commandInfo + activityInfo + gap + progressInfo.
	width := m.width
	if width == 0 {
		width = 120 // Default width if not set.
	}

	leftPart := spin + commandInfo + activityInfo
	leftWidth := lipgloss.Width(leftPart)
	rightWidth := lipgloss.Width(progressInfo)

	// Calculate gap to right-align the progress bar.
	gap := ""
	cellsRemaining := width - leftWidth - rightWidth
	if cellsRemaining > 0 {
		gap = strings.Repeat(" ", cellsRemaining)
	}

	b.WriteString(leftPart + gap + progressInfo)
	b.WriteString("\n\n")

	// Render only completed/errored resources (in-progress ones are shown on the header line).
	resources := m.tracker.GetResources()
	for _, res := range resources {
		if res.State == ResourceStateComplete || res.State == ResourceStateError {
			b.WriteString(m.renderResource(res))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// formatActivityVerb returns a short verb describing the current activity.
func (m Model) formatActivityVerb(op *ResourceOperation) string {
	switch op.State {
	case ResourceStateRefreshing:
		return "Reading"
	case ResourceStateInProgress:
		switch op.Action {
		case "create":
			return "Creating"
		case "update":
			return "Updating"
		case "delete":
			return "Destroying"
		case "read":
			return "Reading"
		default:
			return "Processing"
		}
	default:
		return "Processing"
	}
}

// renderResource renders a single resource line.
func (m Model) renderResource(res *ResourceOperation) string {
	var icon string
	var actionVerb string
	var style lipgloss.Style

	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	switch res.State {
	case ResourceStatePending:
		icon = "○"
		actionVerb = m.formatActionPending(res.Action)
		style = mutedStyle
	case ResourceStateRefreshing:
		icon = m.spinner.View()
		actionVerb = "Refreshing"
		style = warningStyle
	case ResourceStateInProgress:
		icon = m.spinner.View()
		actionVerb = m.formatActionInProgress(res.Action)
		style = warningStyle
	case ResourceStateComplete:
		icon = "✓"
		actionVerb = m.formatActionComplete(res.Action)
		style = successStyle
	case ResourceStateError:
		icon = "✗"
		actionVerb = "Failed"
		style = errorStyle
	}

	// Build timing info.
	var timingStr string
	if res.State == ResourceStateInProgress || res.State == ResourceStateRefreshing {
		elapsed := time.Since(res.StartTime).Seconds()
		timingStr = fmt.Sprintf(" (%.1fs)", elapsed)
	} else if res.State == ResourceStateComplete || res.State == ResourceStateError {
		if res.ElapsedSecs > 0 {
			timingStr = fmt.Sprintf(" (%.1fs)", float64(res.ElapsedSecs))
		} else if !res.EndTime.IsZero() {
			timingStr = fmt.Sprintf(" (%.1fs)", res.EndTime.Sub(res.StartTime).Seconds())
		}
	}

	return fmt.Sprintf("  %s %s %s%s",
		style.Render(icon),
		style.Render(actionVerb),
		res.Address,
		mutedStyle.Render(timingStr),
	)
}

// formatActionPending formats the pending action verb.
func (m Model) formatActionPending(action string) string {
	switch action {
	case "create":
		return "Create"
	case "read":
		return "Read"
	case "update":
		return "Update"
	case "delete":
		return "Destroy"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// formatActionInProgress formats the in-progress action verb.
func (m Model) formatActionInProgress(action string) string {
	switch action {
	case "create":
		return "Creating"
	case "read":
		return "Reading"
	case "update":
		return "Updating"
	case "delete":
		return "Destroying"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// formatActionComplete formats the completed action verb.
func (m Model) formatActionComplete(action string) string {
	switch action {
	case "create":
		return "Created"
	case "read":
		return "Read"
	case "update":
		return "Updated"
	case "delete":
		return "Destroyed"
	case "no-op":
		return "No change"
	default:
		return action
	}
}

// finalView renders the completion state.
func (m Model) finalView() string {
	var b strings.Builder
	elapsed := time.Since(m.startTime).Seconds()

	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	// Determine command name for display.
	// If it's an apply with only deletions, show "Destroy" instead of "Apply".
	command := capitalizeCommand(m.command)
	summary := m.tracker.GetChangeSummary()
	if m.command == "apply" && summary != nil {
		if summary.Changes.Remove > 0 && summary.Changes.Add == 0 && summary.Changes.Change == 0 {
			command = "Destroy"
		}
	}

	// Condensed summary.
	if m.tracker.HasErrors() {
		errorCount := m.tracker.GetErrorCount()
		b.WriteString(atmosui.FormatErrorf("%s `%s/%s` failed: %d error(s) (%.1fs)",
			command,
			m.stack,
			m.component,
			errorCount,
			elapsed,
		))
		b.WriteString("\n")

		// Show error details.
		for _, diag := range m.tracker.GetDiagnostics() {
			if diag.Diagnostic.Severity == "error" {
				b.WriteString(fmt.Sprintf("  %s %s\n",
					errorStyle.Render("Error:"),
					diag.Diagnostic.Summary,
				))
				if diag.Diagnostic.Detail != "" {
					b.WriteString(fmt.Sprintf("    %s\n", mutedStyle.Render(diag.Diagnostic.Detail)))
				}
			}
		}

		// Show failed resources.
		for _, res := range m.tracker.GetResources() {
			if res.State == ResourceStateError {
				b.WriteString(fmt.Sprintf("  %s %s: %s\n",
					errorStyle.Render("✗"),
					res.Address,
					res.Error,
				))
			}
		}
	} else {
		// Success summary (reuse summary fetched above for command detection).
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
		if summary != nil && (summary.Changes.Add == 0 && summary.Changes.Change == 0 && summary.Changes.Remove == 0) {
			// No changes - include in markdown for bold rendering.
			b.WriteString(atmosui.FormatSuccessf("%s `%s/%s` completed (*no changes*)",
				command,
				m.stack,
				m.component,
			))
			b.WriteString(dimStyle.Render(fmt.Sprintf(" (%.1fs)", elapsed)))
		} else {
			// Render base message first.
			b.WriteString(atmosui.FormatSuccessf("%s `%s/%s` completed",
				command,
				m.stack,
				m.component,
			))
			b.WriteString(dimStyle.Render(fmt.Sprintf(" (%.1fs)", elapsed)))
		}
		b.WriteString("\n")
	}

	// Show warnings.
	for _, diag := range m.tracker.GetDiagnostics() {
		if diag.Diagnostic.Severity == "warning" {
			b.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow)).Render("Warning:"),
				diag.Diagnostic.Summary,
			))
		}
	}

	return b.String()
}

// capitalizeCommand returns the command with the first letter capitalized.
func capitalizeCommand(cmd string) string {
	if len(cmd) == 0 {
		return cmd
	}
	return strings.ToUpper(cmd[:1]) + cmd[1:]
}

// GetExitCode returns the exit code after completion.
func (m *Model) GetExitCode() int {
	return m.exitCode
}

// GetError returns any error that occurred.
func (m *Model) GetError() error {
	return m.err
}

// GetTracker returns the resource tracker.
func (m *Model) GetTracker() *ResourceTracker {
	return m.tracker
}
