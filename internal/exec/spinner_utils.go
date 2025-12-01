package exec

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

type modelSpinner struct {
	spinner spinner.Model
	message string
}

func (m modelSpinner) Init() tea.Cmd {
	return m.spinner.Tick
}

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

func (m modelSpinner) View() string {
	return fmt.Sprintf("\r%s %s", m.spinner.View(), m.message)
}

// NewSpinner initializes a spinner and returns a pointer to a tea.Program.
func NewSpinner(message string) *tea.Program {
	s := spinner.New()
	styles := theme.GetCurrentStyles()
	s.Style = styles.Link

	var opts []tea.ProgramOption
	if !term.IsTTYSupportForStdout() {
		// Workaround for non-TTY environments.
		opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
		log.Debug("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
		fmt.Fprintln(os.Stderr, message)
	}

	p := tea.NewProgram(modelSpinner{
		spinner: s,
		message: message,
	}, opts...)

	return p
}

// RunSpinner executes the spinner program in a goroutine.
func RunSpinner(p *tea.Program, spinnerChan chan struct{}, message string) {
	go func() {
		defer close(spinnerChan)
		if _, err := p.Run(); err != nil {
			// If there's any error running the spinner, print the message and the error.
			fmt.Println(message)
			log.Error("Failed to run spinner:", "error", err)
		}
	}()
}

// StopSpinner stops the spinner program and waits for the completion.
func StopSpinner(p *tea.Program, spinnerChan chan struct{}) {
	p.Quit()
	<-spinnerChan
}
