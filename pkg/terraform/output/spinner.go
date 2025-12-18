package output

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

type modelSpinner struct {
	spinner spinner.Model
	message string
}

//nolint:gocritic // bubbletea models must be passed by value.
func (m modelSpinner) Init() tea.Cmd {
	return m.spinner.Tick
}

//nolint:gocritic // bubbletea models must be passed by value.
func (m modelSpinner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

//nolint:gocritic // bubbletea models must be passed by value.
func (m modelSpinner) View() string {
	return fmt.Sprintf("%s%s %s", terminal.EscCarriageReturn, m.spinner.View(), m.message)
}

// NewSpinner creates a tea.Program that displays an animated spinner with the provided message.
// It applies the current UI spinner style. If stdout lacks TTY support, the program is configured
// without a renderer and without input; a debug message is logged and the message is written to stderr
// as a fallback.
func NewSpinner(message string) *tea.Program {
	defer perf.Track(nil, "output.NewSpinner")()

	s := spinner.New()
	s.Style = theme.GetCurrentStyles().Spinner

	var opts []tea.ProgramOption
	if !term.IsTTYSupportForStdout() {
		// Workaround for non-TTY environments.
		opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
		log.Debug("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
		_ = ui.Writeln(message)
	}

	p := tea.NewProgram(modelSpinner{
		spinner: s,
		message: message,
	}, opts...)

	return p
}

// RunSpinner executes the spinner program in a goroutine.
func RunSpinner(p *tea.Program, spinnerChan chan struct{}, message string) {
	defer perf.Track(nil, "output.RunSpinner")()

	go func() {
		defer close(spinnerChan)
		if _, err := p.Run(); err != nil {
			// If there's any error running the spinner, output the message and log the error.
			_ = ui.Writeln(message)
			log.Error("Failed to run spinner:", "error", err)
		}
	}()
}

// StopSpinner stops the spinner program and waits for the completion.
func StopSpinner(p *tea.Program, spinnerChan chan struct{}) {
	defer perf.Track(nil, "output.StopSpinner")()

	p.Quit()
	<-spinnerChan
}
