package workflow

import (
	codeviewport "github.com/cloudposse/atmos/internal/tui/components/code_view"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the properties of the UI
type Model struct {
	code codeviewport.Model
}

// New creates a new instance of the model
func New() Model {
	codeModel := codeviewport.New(true, true, lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"})

	return Model{
		code: codeModel,
	}
}

// Init initializes the UI
func (m *Model) Init() tea.Cmd {
	return nil
}

// SetContent sets content
func (m *Model) SetContent(content string, extension string) tea.Cmd {
	return m.code.SetCode(content, extension)
}

// Update handles all UI interactions
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.code.SetSize(msg.Width, msg.Height)

		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			cmds = append(cmds, tea.Quit)
		}
	}

	mod, cmd := m.code.Update(msg)
	m.code = *mod
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View returns a string representation of the UI
func (m *Model) View() string {
	return m.code.View()
}
