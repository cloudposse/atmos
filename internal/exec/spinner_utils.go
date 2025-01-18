package exec

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type modelSpinner struct {
	spinner spinner.Model
	message string
}

func (m modelSpinner) Init() tea.Cmd {
	// Check if we're running in a terminal
	if !isTerminal() {
		return tea.Quit
	}
	return m.spinner.Tick
}

// isTerminal checks if stdout is a terminal
func isTerminal() bool {
	fileInfo, _ := os.Stdout.Stat()
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
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
