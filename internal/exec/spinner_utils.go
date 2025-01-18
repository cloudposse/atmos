package exec

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
