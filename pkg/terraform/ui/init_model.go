package ui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/cloudposse/atmos/pkg/perf"
	atmosui "github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	initMaxLines = 6 // Maximum lines to show in viewport.

	// Line display constants.
	initMaxLineWidth   = 70 // Maximum width for displayed lines.
	initTruncatedWidth = 67 // Width after truncation (leaves room for "...").

	// ANSI escape sequences for terminal control.
	initClearToEOL = "\x1b[K" // Clear from cursor to end of line.
)

// InitModel is the bubbletea model for streaming terraform init/workspace output.
type InitModel struct {
	spinner    spinner.Model
	scanner    *bufio.Scanner
	lines      []string
	currentOp  string // Current operation (e.g., "Initializing the backend...")
	done       bool
	err        error
	exitCode   int
	startTime  time.Time
	component  string
	stack      string
	subCommand string // "init" or "workspace".
	workspace  string // Workspace name (for workspace commands).
	clock      Clock  // Clock for time operations (injectable for testing).
}

// InitModelOption configures an InitModel.
type InitModelOption func(*InitModel)

// WithInitClock sets the clock implementation for time operations.
func WithInitClock(c Clock) InitModelOption {
	return func(m *InitModel) {
		m.clock = c
	}
}

// initLineMsg wraps a line from init output.
type initLineMsg struct {
	line string
}

// initDoneMsg signals init completion.
type initDoneMsg struct {
	exitCode int
	err      error
}

// NewInitModel creates a new init/workspace streaming model.
func NewInitModel(component, stack, subCommand, workspace string, reader io.Reader, opts ...InitModelOption) *InitModel {
	defer perf.Track(nil, "terraform.ui.NewInitModel")()

	// Use MiniDot spinner for init/workspace (more subtle, different from plan/apply).
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	// Create scanner with increased buffer size for large terraform init output.
	scanner := bufio.NewScanner(reader)
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	m := &InitModel{
		spinner:    s,
		scanner:    scanner,
		lines:      make([]string, 0),
		component:  component,
		stack:      stack,
		subCommand: subCommand,
		workspace:  workspace,
		clock:      defaultClock(),
	}

	// Apply options.
	for _, opt := range opts {
		opt(m)
	}

	// Set startTime using the clock (after options are applied).
	m.startTime = m.clock.Now()

	return m
}

// Init initializes the model.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m InitModel) Init() tea.Cmd {
	defer perf.Track(nil, "terraform.ui.InitModel.Init")()

	return tea.Batch(
		m.spinner.Tick,
		m.readNextLine(),
	)
}

// readNextLine reads the next line from the scanner.
func (m *InitModel) readNextLine() tea.Cmd {
	return func() tea.Msg {
		if m.scanner.Scan() {
			return initLineMsg{line: m.scanner.Text()}
		}
		if err := m.scanner.Err(); err != nil {
			return initDoneMsg{exitCode: 1, err: err}
		}
		return initDoneMsg{exitCode: 0, err: nil}
	}
}

// Update handles messages.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m InitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case initLineMsg:
		// Strip ANSI codes from terraform output to prevent display corruption.
		line := strings.TrimSpace(ansi.Strip(msg.line))
		if line != "" {
			// Track current operation for display.
			if strings.HasPrefix(line, "Initializing") {
				m.currentOp = line
			} else if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
				// Provider/module operations - add to viewport.
				m.lines = append(m.lines, line)
				// Keep only the last N lines.
				if len(m.lines) > initMaxLines {
					m.lines = m.lines[len(m.lines)-initMaxLines:]
				}
			} else if strings.Contains(line, "successfully initialized") {
				m.currentOp = "Initialized successfully"
			}
		}
		return m, m.readNextLine()

	case initDoneMsg:
		m.done = true
		m.exitCode = msg.exitCode
		m.err = msg.err
		return m, tea.Quit
	}

	return m, nil
}

// View renders the model.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m InitModel) View() string {
	if m.done {
		// Use carriage return and clear to end of line to prevent artifacts.
		return "\r" + initClearToEOL + m.renderComplete()
	}
	return m.renderProgress()
}

func (m *InitModel) renderProgress() string {
	var b strings.Builder

	elapsed := m.clock.Since(m.startTime).Seconds()
	action := m.formatAction()

	// Header line with spinner.
	b.WriteString(fmt.Sprintf("%s %s %s/%s (%.1fs)\n",
		m.spinner.View(),
		action,
		m.stack,
		m.component,
		elapsed,
	))

	// Show current operation.
	if m.currentOp != "" {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render(m.currentOp)))
	}

	// Show recent provider/module lines (dimmed).
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	for _, line := range m.lines {
		// Truncate long lines using rune-aware width to handle multi-byte UTF-8.
		if runewidth.StringWidth(line) > initMaxLineWidth {
			line = runewidth.Truncate(line, initTruncatedWidth, "...")
		}
		b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(line)))
	}

	return b.String()
}

func (m *InitModel) renderComplete() string {
	elapsed := m.clock.Since(m.startTime).Seconds()
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))

	// Format the action description.
	action := m.formatAction()

	if m.err != nil || m.exitCode != 0 {
		return atmosui.FormatErrorf("%s `%s/%s` failed",
			action,
			m.stack,
			m.component,
		) + dimStyle.Render(fmt.Sprintf(" (%.1fs)", elapsed)) + "\n"
	}

	// For workspace command, "Selected" already implies completion, so don't add "completed".
	if m.subCommand == "workspace" {
		return atmosui.FormatSuccessf("%s `%s/%s`",
			action,
			m.stack,
			m.component,
		) + dimStyle.Render(fmt.Sprintf(" (%.1fs)", elapsed)) + "\n"
	}

	return atmosui.FormatSuccessf("%s `%s/%s` completed",
		action,
		m.stack,
		m.component,
	) + dimStyle.Render(fmt.Sprintf(" (%.1fs)", elapsed)) + "\n"
}

// formatAction returns a human-readable action description.
func (m *InitModel) formatAction() string {
	switch m.subCommand {
	case "init":
		return "Init"
	case "workspace":
		if m.workspace != "" {
			return fmt.Sprintf("Selected `%s` workspace for", m.workspace)
		}
		return "Selected workspace for"
	default:
		// Capitalize first letter.
		if len(m.subCommand) > 0 {
			return strings.ToUpper(m.subCommand[:1]) + m.subCommand[1:]
		}
		return m.subCommand
	}
}

// GetError returns any error that occurred.
func (m *InitModel) GetError() error {
	return m.err
}

// GetExitCode returns the exit code.
func (m *InitModel) GetExitCode() int {
	return m.exitCode
}
