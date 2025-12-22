package spinner

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const newline = "\n"

// ExecWithSpinner runs an operation with a spinner UI.
// ProgressMsg is shown while operation is running (e.g., "Starting container").
// CompletedMsg is shown when operation completes successfully (e.g., "Started container").
func ExecWithSpinner(progressMsg, completedMsg string, operation func() error) error {
	defer perf.Track(nil, "spinner.ExecWithSpinner")()

	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show result.
		err := operation()
		if err != nil {
			return err
		}
		_ = ui.Success(completedMsg)
		return nil
	}

	// TTY available - use spinner.
	model := newSpinnerModel(progressMsg, completedMsg)

	// Use inline mode - output to stderr, no alternate screen.
	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	// Run operation in background.
	go func() {
		err := operation()
		p.Send(opCompleteMsg{err: err})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("spinner error: %w", err)
	}

	if m, ok := finalModel.(spinnerModel); ok && m.err != nil {
		return m.err
	}

	return nil
}

// spinnerModel is a simple spinner model for long-running operations.
type spinnerModel struct {
	spinner      spinner.Model
	progressMsg  string // Message shown during operation (e.g., "Starting container").
	completedMsg string // Message shown when done (e.g., "Started container").
	done         bool
	err          error
}

type opCompleteMsg struct {
	err error
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case opCompleteMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m spinnerModel) View() string {
	if m.done {
		if m.err != nil {
			// Show error indicator with reset line to clear spinner.
			// Use FormatError for proper markdown rendering.
			return terminal.EscResetLine + ui.FormatError(m.progressMsg) + newline
		}
		// Show completed message with checkmark.
		// Use FormatSuccess for proper markdown rendering.
		return terminal.EscResetLine + ui.FormatSuccess(m.completedMsg) + newline
	}
	// Show progress message with spinner.
	return fmt.Sprintf("%s%s %s", terminal.EscResetLine, m.spinner.View(), m.progressMsg)
}

func newSpinnerModel(progressMsg, completedMsg string) spinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	return spinnerModel{
		spinner:      s,
		progressMsg:  progressMsg,
		completedMsg: completedMsg,
	}
}

// ExecWithSpinnerDynamic runs an operation with a spinner UI where the completion message
// is determined dynamically by the operation. The operation returns both the completion
// message and any error. This is useful when the completion message depends on the result
// of the operation (e.g., displaying refs that were compared).
func ExecWithSpinnerDynamic(progressMsg string, operation func() (string, error)) error {
	defer perf.Track(nil, "spinner.ExecWithSpinnerDynamic")()

	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show result.
		completedMsg, err := operation()
		if err != nil {
			return err
		}
		// Show the dynamic completion message if provided, otherwise use progress message.
		if completedMsg != "" {
			_ = ui.Success(completedMsg)
		} else {
			_ = ui.Success(progressMsg)
		}
		return nil
	}

	// TTY available - use spinner with dynamic completion message.
	model := newDynamicSpinnerModel(progressMsg)

	// Use inline mode - output to stderr, no alternate screen.
	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	// Run operation in background.
	go func() {
		completedMsg, err := operation()
		p.Send(opCompleteDynamicMsg{completedMsg: completedMsg, err: err})
	}()

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("spinner error: %w", err)
	}

	if m, ok := finalModel.(dynamicSpinnerModel); ok && m.err != nil {
		return m.err
	}

	return nil
}

// dynamicSpinnerModel is a spinner model where completion message is set dynamically.
type dynamicSpinnerModel struct {
	spinner      spinner.Model
	progressMsg  string // Message shown during operation.
	completedMsg string // Message shown when done (set dynamically).
	done         bool
	err          error
}

type opCompleteDynamicMsg struct {
	completedMsg string
	err          error
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case opCompleteDynamicMsg:
		m.done = true
		m.completedMsg = msg.completedMsg
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m dynamicSpinnerModel) View() string {
	if m.done {
		if m.err != nil {
			// Show error indicator with reset line to clear spinner.
			// Use FormatError for proper markdown rendering.
			return terminal.EscResetLine + ui.FormatError(m.progressMsg) + newline
		}
		// Show completed message with checkmark.
		// Use FormatSuccess for proper markdown rendering.
		displayMsg := m.completedMsg
		if displayMsg == "" {
			displayMsg = m.progressMsg
		}
		return terminal.EscResetLine + ui.FormatSuccess(displayMsg) + newline
	}
	// Show progress message with spinner.
	return fmt.Sprintf("%s%s %s", terminal.EscResetLine, m.spinner.View(), m.progressMsg)
}

func newDynamicSpinnerModel(progressMsg string) dynamicSpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	return dynamicSpinnerModel{
		spinner:     s,
		progressMsg: progressMsg,
	}
}

// Spinner provides a start/stop API for long-running operations where multiple
// sequential steps need to run while the spinner is displayed. For single operations,
// prefer ExecWithSpinner or ExecWithSpinnerDynamic instead.
//
// Example usage:
//
//	s := spinner.New("Processing...")
//	s.Start()
//	defer s.Stop()
//
//	// Do multiple operations while spinner runs...
//	step1()
//	step2()
//
//	s.Success("Processing complete")
type Spinner struct {
	progressMsg string
	program     *tea.Program
	done        chan struct{}
	isTTY       bool
}

// New creates a new Spinner with the given progress message.
func New(progressMsg string) *Spinner {
	defer perf.Track(nil, "spinner.New")()

	return &Spinner{
		progressMsg: progressMsg,
		isTTY:       term.IsTTYSupportForStdout(),
	}
}

// Start begins displaying the spinner. Call Stop() when done.
func (s *Spinner) Start() {
	if !s.isTTY {
		// No TTY - no spinner to show.
		return
	}

	model := newManualSpinnerModel(s.progressMsg)
	s.program = tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		_, _ = s.program.Run()
	}()
}

// Stop stops the spinner without displaying a completion message.
// Use Success() or Error() instead to show a completion status.
// Stop is idempotent and safe to call multiple times.
func (s *Spinner) Stop() {
	if s.program == nil {
		return
	}
	s.program.Send(manualStopMsg{})
	<-s.done
	s.program = nil
}

// Success stops the spinner and displays a success message with checkmark.
// Success is idempotent and safe to call multiple times.
func (s *Spinner) Success(message string) {
	if !s.isTTY {
		_ = ui.Success(message)
		return
	}
	if s.program == nil {
		_ = ui.Success(message)
		return
	}
	s.program.Send(manualStopMsg{message: message, success: true})
	<-s.done
	s.program = nil
}

// Error stops the spinner and displays an error message with xmark.
// Error is idempotent and safe to call multiple times.
func (s *Spinner) Error(message string) {
	if !s.isTTY {
		_ = ui.Error(message)
		return
	}
	if s.program == nil {
		_ = ui.Error(message)
		return
	}
	s.program.Send(manualStopMsg{message: message, success: false})
	<-s.done
	s.program = nil
}

// manualSpinnerModel is a spinner that runs until explicitly stopped.
type manualSpinnerModel struct {
	spinner     spinner.Model
	progressMsg string
	finalMsg    string
	success     bool
	done        bool
}

type manualStopMsg struct {
	message string
	success bool
}

func newManualSpinnerModel(progressMsg string) manualSpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.GetCurrentStyles().Spinner
	return manualSpinnerModel{
		spinner:     s,
		progressMsg: progressMsg,
	}
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case manualStopMsg:
		m.done = true
		m.finalMsg = msg.message
		m.success = msg.success
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

//nolint:gocritic // bubbletea models must be passed by value
func (m manualSpinnerModel) View() string {
	if m.done {
		if m.finalMsg == "" {
			// Just clear the line if no message.
			return terminal.EscResetLine
		}
		// Use FormatSuccess/FormatError for proper markdown rendering.
		if m.success {
			return terminal.EscResetLine + ui.FormatSuccess(m.finalMsg) + newline
		}
		return terminal.EscResetLine + ui.FormatError(m.finalMsg) + newline
	}
	return fmt.Sprintf("%s%s %s", terminal.EscResetLine, m.spinner.View(), m.progressMsg)
}
