package code_view

import (
	"fmt"
	u "github.com/cloudposse/atmos/internal/tui/utils"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultPadding = 1
)

type Model struct {
	Viewport           viewport.Model
	HighlightedContent string
	SyntaxTheme        string
}

// New creates a new instance of the model
func New(syntaxTheme string) Model {
	viewPort := viewport.New(0, 0)

	viewPort.Style = lipgloss.NewStyle().
		PaddingLeft(defaultPadding).
		PaddingRight(defaultPadding)

	return Model{
		Viewport:    viewPort,
		SyntaxTheme: syntaxTheme,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return nil
}

// SetContent sets content
func (m *Model) SetContent(content string, language string) {
	highlighted, _ := u.HighlightCode(content, language, m.SyntaxTheme)
	m.HighlightedContent = highlighted

	m.Viewport.MouseWheelEnabled = true

	m.Viewport.SetContent(lipgloss.NewStyle().
		Width(m.Viewport.Width).
		Height(m.Viewport.Height).
		Render(highlighted))
}

// SetSyntaxTheme sets the syntax theme of the rendered code
func (m *Model) SetSyntaxTheme(theme string) {
	m.SyntaxTheme = theme
}

// SetSize sets the size of the view
func (m *Model) SetSize(width int, height int) {
	m.Viewport.Width = width
	m.Viewport.Height = height

	m.Viewport.SetContent(lipgloss.NewStyle().
		Width(m.Viewport.Width).
		Height(m.Viewport.Height).
		Render(m.HighlightedContent))
}

// Update handles updating the UI
func (m *Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return *m, cmd
}

// View returns a string representation of the model
func (m *Model) View() string {
	m.Viewport.Style = lipgloss.NewStyle().
		PaddingLeft(defaultPadding).
		PaddingRight(defaultPadding)

	return m.Viewport.View()
}

func (m *Model) CursorUp() {
	lines := m.Viewport.LineUp(1)
	m.Viewport.SetContent(fmt.Sprintf("%v", lines))
}

func (m *Model) CursorDown() {
	lines := m.Viewport.LineDown(1)
	m.Viewport.SetContent(fmt.Sprintf("%v", lines))
}
