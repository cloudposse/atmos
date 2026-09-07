package ui

import (
	"errors"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	progressBarWidth = 40
	tickInterval     = 100 * time.Millisecond

	// ANSI escape sequences for terminal control.
	clearToEOL = "\x1b[K" // Clear from cursor to end of line.
	clearToEOS = "\x1b[J" // Clear from cursor to end of screen.
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
	cancelled bool
	startTime time.Time
	component string // Component name for display.
	stack     string // Stack name for display.
	command   string // "plan", "apply", "init", "refresh".
	clock     Clock  // Clock for time operations (injectable for testing).
}

// ModelOption configures a Model.
type ModelOption func(*Model)

// WithClock sets the clock implementation for time operations.
func WithClock(c Clock) ModelOption {
	return func(m *Model) {
		m.clock = c
	}
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

// tickMsg for periodic updates.
type tickMsg time.Time

// NewModel creates a new streaming model.
func NewModel(component, stack, command string, reader io.Reader, opts ...ModelOption) *Model {
	defer perf.Track(nil, "terraform.ui.NewModel")()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(progressBarWidth),
		progress.WithoutPercentage(),
	)

	m := &Model{
		tracker:   NewResourceTracker(),
		parser:    NewParser(reader),
		reader:    reader,
		spinner:   s,
		progress:  p,
		component: component,
		stack:     stack,
		command:   command,
		clock:     defaultClock(),
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
func (m Model) Init() tea.Cmd {
	defer perf.Track(nil, "terraform.ui.Model.Init")()

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

// isQuitKey returns true if key should quit the TUI (used by both Model and InitModel).
func isQuitKey(key string) bool {
	return key == "ctrl+c" || key == "q"
}

// handleParsedMessage applies a parsed terraform message to the tracker (if valid) and
// returns the command to listen for the next message.
func (m *Model) handleParsedMessage(result *ParseResult) tea.Cmd {
	if result.Err == nil && result.Message != nil {
		m.tracker.HandleMessage(result.Message)
	}
	return m.listenForMessages()
}

// Update handles messages.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer perf.Track(nil, "terraform.ui.Model.Update")()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = min(progressBarWidth, msg.Width-10)
		return m, nil

	case tea.KeyMsg:
		if isQuitKey(msg.String()) {
			// Mark cancelled (as opposed to doneMsg, which means the command finished on
			// its own) so the caller knows to kill the still-running subprocess instead of
			// waiting for it to finish invisibly in the background.
			m.done = true
			m.cancelled = true
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
		return m, m.handleParsedMessage(msg.result)

	case doneMsg:
		m.exitCode = msg.exitCode
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View renders the UI.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) View() string {
	defer perf.Track(nil, "terraform.ui.Model.View")()

	if m.done {
		// Bubbletea has already moved the cursor to the top of the previous frame before
		// it writes this view, so the stale progress block is entirely below the cursor:
		// erase from here to the end of the screen and draw the summary in its place.
		//
		// This must not move the cursor up. An earlier version walked up one row per
		// progress line to clear them, on top of bubbletea's own repositioning, so it
		// erased that many rows *above* the block -- the command the user typed, the
		// init and workspace lines, whatever was there -- and left the summary stranded
		// high on the screen with a blank tail below it.
		return "\r" + clearToEOS + m.finalView()
	}
	return m.progressView()
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

// Cancelled reports whether the user explicitly quit (Ctrl-C/q) rather than the underlying
// terraform command completing on its own. Value receiver: bubbletea returns models by
// value, so this must be callable on the Model value stored in the tea.Model interface.
//
//nolint:gocritic // bubbletea models must be passed by value
func (m Model) Cancelled() bool {
	return m.cancelled
}
