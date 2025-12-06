package spinner

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// ExecWithSpinner runs an operation with a spinner UI.
// ProgressMsg is shown while operation is running (e.g., "Starting container").
// CompletedMsg is shown when operation completes successfully (e.g., "Started container").
func ExecWithSpinner(progressMsg, completedMsg string, operation func() error) error {
	// Check if TTY is available.
	isTTY := term.IsTTYSupportForStdout()

	if !isTTY {
		// No TTY - just run the operation and show simple output on one line.
		_ = ui.Writef("%s... ", progressMsg)
		err := operation()
		if err != nil {
			_ = ui.Writeln("")
			return err
		}
		checkmark := theme.GetCurrentStyles().Checkmark.String()
		_ = ui.Writeln(checkmark)
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
			xmark := theme.GetCurrentStyles().XMark.String()
			return fmt.Sprintf("%s%s %s\n", terminal.EscResetLine, xmark, m.progressMsg)
		}
		// Show completed message with checkmark.
		checkmark := theme.GetCurrentStyles().Checkmark.String()
		return fmt.Sprintf("%s%s %s\n", terminal.EscResetLine, checkmark, m.completedMsg)
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
