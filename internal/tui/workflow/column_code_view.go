package workflow

import (
	codeview "github.com/cloudposse/atmos/internal/tui/components/code_view"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	mouseZone "github.com/lrstanley/bubblezone"
)

// codeColumnView represents the properties of the UI
type codeColumnView struct {
	id      string
	code    codeview.Model
	focused bool
}

func (c *codeColumnView) Focus() {
	c.focused = true
}

func (c *codeColumnView) Blur() {
	c.focused = false
}

func (c *codeColumnView) Focused() bool {
	return c.focused
}

// newCodeViewColumn creates a new instance of the model
func newCodeViewColumn(syntaxTheme string) codeColumnView {
	codeModel := codeview.New(true, true, lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}, syntaxTheme)

	return codeColumnView{
		id:   mouseZone.NewPrefix(),
		code: codeModel,
	}
}

// Init initializes the UI
func (m *codeColumnView) Init() tea.Cmd {
	return nil
}

// SetContent sets content
func (m *codeColumnView) SetContent(content string, language string) tea.Cmd {
	return m.code.SetContent(content, language)
}

// Update handles all UI interactions
func (m *codeColumnView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.code.SetSize(msg.Width, msg.Height)
		return m, nil
	}

	mod, cmd := m.code.Update(msg)
	m.code = *mod
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View returns a string representation of the UI
func (m *codeColumnView) View() string {
	return m.code.View()
}
