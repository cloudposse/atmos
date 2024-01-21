package workflow

import (
	"bytes"
	"fmt"

	"github.com/alecthomas/chroma/quick"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type syntaxMsg string
type errorMsg error

const (
	padding = 1
)

// Highlight returns a syntax highlighted string of text.
func Highlight(content, extension, syntaxTheme string) (string, error) {
	buf := new(bytes.Buffer)
	if err := quick.Highlight(buf, content, extension, "terminal256", syntaxTheme); err != nil {
		return "", fmt.Errorf("%w", err)
	}

	return buf.String(), nil
}

// Model represents the properties of a code bubble.
type Model struct {
	Viewport           viewport.Model
	BorderColor        lipgloss.AdaptiveColor
	Borderless         bool
	Active             bool
	Filename           string
	HighlightedContent string
	SyntaxTheme        string
}

// New creates a new instance of the model.
func New(active, borderless bool, borderColor lipgloss.AdaptiveColor) Model {
	viewPort := viewport.New(0, 0)
	border := lipgloss.NormalBorder()

	if borderless {
		border = lipgloss.HiddenBorder()
	}

	viewPort.Style = lipgloss.NewStyle().
		PaddingLeft(padding).
		PaddingRight(padding).
		Border(border).
		BorderForeground(borderColor)

	return Model{
		Viewport:    viewPort,
		Borderless:  borderless,
		Active:      active,
		BorderColor: borderColor,
		SyntaxTheme: "dracula",
	}
}

// Init initializes the code bubble.
func (m Model) Init() tea.Cmd {
	return nil
}

// SetFileName sets current file to highlight.
func (m *Model) SetFileName(filename string) tea.Cmd {
	m.Filename = filename

	return nil
}

// SetIsActive sets if the bubble is currently active.
func (m *Model) SetIsActive(active bool) {
	m.Active = active
}

// SetBorderColor sets the current color of the border.
func (m *Model) SetBorderColor(color lipgloss.AdaptiveColor) {
	m.BorderColor = color
}

// SetSyntaxTheme sets the syntax theme of the rendered code.
func (m *Model) SetSyntaxTheme(theme string) {
	m.SyntaxTheme = theme
}

// SetBorderless sets weather or not to show the border.
func (m *Model) SetBorderless(borderless bool) {
	m.Borderless = borderless
}

// SetSize sets the size of the bubble.
func (m *Model) SetSize(w, h int) {
	m.Viewport.Width = w
	m.Viewport.Height = h

	m.Viewport.SetContent(lipgloss.NewStyle().
		Width(m.Viewport.Width).
		Height(m.Viewport.Height).
		Render(m.HighlightedContent))
}

// GotoTop jumps to the top of the viewport.
func (m *Model) GotoTop() {
	m.Viewport.GotoTop()
}

// Update handles updating the UI of a code bubble.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case syntaxMsg:
		m.Filename = ""
		m.HighlightedContent = lipgloss.NewStyle().
			Width(m.Viewport.Width).
			Height(m.Viewport.Height).
			Render(string(msg))

		m.Viewport.SetContent(m.HighlightedContent)

		return m, nil
	case errorMsg:
		m.Filename = ""
		m.HighlightedContent = lipgloss.NewStyle().
			Width(m.Viewport.Width).
			Height(m.Viewport.Height).
			Render("Error: " + msg.Error())

		m.Viewport.SetContent(m.HighlightedContent)

		return m, nil
	}

	if m.Active {
		m.Viewport, cmd = m.Viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View returns a string representation of the code bubble.
func (m Model) View() string {
	border := lipgloss.NormalBorder()

	if m.Borderless {
		border = lipgloss.HiddenBorder()
	}

	m.Viewport.Style = lipgloss.NewStyle().
		PaddingLeft(padding).
		PaddingRight(padding).
		Border(border).
		BorderForeground(m.BorderColor)

	return m.Viewport.View()
}
