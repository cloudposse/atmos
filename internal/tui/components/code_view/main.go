package code_view

import (
	u "github.com/cloudposse/atmos/internal/tui/utils"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	Viewport           viewport.Model
	HighlightedContent string
	SyntaxTheme        string
	IsMarkdown         bool
}

// New creates a new instance of the model.
func New(syntaxTheme string) Model {
	viewPort := viewport.New(0, 0)

	viewPort.Style = lipgloss.NewStyle().
		PaddingLeft(0).
		PaddingRight(0)

	return Model{
		Viewport:    viewPort,
		SyntaxTheme: syntaxTheme,
		IsMarkdown:  false,
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// SetContent sets content.
func (m *Model) SetContent(content string, language string) {
	var rendered string
	var err error

	if language == "markdown" || language == "md" {
		m.IsMarkdown = true
		rendered, err = u.RenderMarkdown(content, "")
		if err != nil {
			// Fallback to plain text if markdown rendering fails
			rendered = content
		}
	} else {
		m.IsMarkdown = false
		rendered, err = u.HighlightCode(content, language, m.SyntaxTheme)
		if err != nil {
			// Fallback to plain text if syntax highlighting fails
			rendered = content
		}
	}

	m.HighlightedContent = rendered

	m.Viewport.ViewUp()
	m.Viewport.MouseWheelEnabled = true

	m.Viewport.SetContent(lipgloss.NewStyle().
		Width(m.Viewport.Width).
		Height(m.Viewport.Height).
		Render(rendered))
}

// SetSyntaxTheme sets the syntax theme.
func (m *Model) SetSyntaxTheme(theme string) {
	m.SyntaxTheme = theme
}

// SetSize sets the size of the view.
func (m *Model) SetSize(width int, height int) {
	m.Viewport.Width = width
	m.Viewport.Height = height

	m.Viewport.SetContent(lipgloss.NewStyle().
		Width(m.Viewport.Width).
		Height(m.Viewport.Height).
		Render(m.HighlightedContent))
}

// Update handles updating the UI.
func (m *Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return *m, cmd
}

// View returns a string representation of the model.
func (m *Model) View() string {
	m.Viewport.Style = lipgloss.NewStyle().
		PaddingLeft(0).
		PaddingRight(0)

	return m.Viewport.View()
}

func (m *Model) CursorUp() {
	m.Viewport.LineUp(1)
}

func (m *Model) CursorDown() {
	m.Viewport.LineDown(1)
}
