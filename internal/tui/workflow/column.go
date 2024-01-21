package workflow

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	codeview "github.com/cloudposse/atmos/internal/tui/components/code_view"
	mouseZone "github.com/lrstanley/bubblezone"
)

type columnView struct {
	id       string
	focused  bool
	viewType string
	list     list.Model
	codeView codeview.Model
	height   int
	width    int
}

func (c *columnView) CursorUp() {
	if c.viewType == "list" {
		c.list.CursorUp()
	}
}

func (c *columnView) CursorDown() {
	if c.viewType == "list" {
		c.list.CursorDown()
	}
}

func (c *columnView) Focus() {
	c.focused = true
}

func (c *columnView) Blur() {
	c.focused = false
}

func (c *columnView) Focused() bool {
	return c.focused
}

func newColumn(columnPointer int, viewType string) columnView {
	var focused bool
	if columnPointer == 0 {
		focused = true
	}

	var defaultList list.Model
	var codeView codeview.Model

	if viewType == "list" {
		defaultList = list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
		defaultList.SetShowHelp(false)
	}

	if viewType == "codeView" {
		codeView = codeview.New(true, true, lipgloss.AdaptiveColor{Light: "#000000", Dark: "#ffffff"}, "solarized-dark256")
	}

	return columnView{
		id:       mouseZone.NewPrefix(),
		focused:  focused,
		viewType: viewType,
		list:     defaultList,
		codeView: codeView,
	}
}

// Init does initial setup
func (c *columnView) Init() tea.Cmd {
	return nil
}

// Update handles all the I/O
func (c *columnView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		c.setSize(message.Width, message.Height)

		if c.viewType == "list" {
			c.list.SetSize(message.Width/4, message.Height/3)
		}
		if c.viewType == "codeView" {
			c.codeView.SetSize(message.Width/4, message.Height/3)
		}
	}

	if c.viewType == "list" {
		c.list, cmd = c.list.Update(msg)
	}
	if c.viewType == "codeView" {
		c.codeView, cmd = c.codeView.Update(msg)
	}

	return c, cmd
}

func (c *columnView) View() string {
	if c.viewType == "list" {
		return mouseZone.Mark(c.id, c.getStyle().Render(c.list.View()))
	}
	if c.viewType == "codeView" {
		return mouseZone.Mark(c.id, c.getStyle().Render(c.codeView.View()))
	}

	return ""
}

func (c *columnView) setSize(width, height int) {
	c.width = width / 4
}

func (c *columnView) getStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Padding(1, 2).Height(c.height).Width(c.width)

	if c.Focused() {
		s.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	} else {
		s.Border(lipgloss.HiddenBorder())
	}

	return s
}

// SetContent sets content
func (c *columnView) SetContent(content string, language string) tea.Cmd {
	if c.viewType == "codeView" {
		return c.codeView.SetContent(content, language)
	}
	return nil
}
