package workflow

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	codeview "github.com/cloudposse/atmos/internal/tui/components/code_view"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	mouseZone "github.com/lrstanley/bubblezone"
)

const (
	listViewType  = "listView"
	listViewType2 = "listView2"
	codeViewType  = "codeView"
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
	if c.viewType == listViewType || c.viewType == listViewType2 {
		c.list.CursorUp()
	}
	if c.viewType == codeViewType {
		c.codeView.CursorUp()
	}
}

func (c *columnView) CursorDown() {
	if c.viewType == listViewType || c.viewType == listViewType2 {
		c.list.CursorDown()
	}
	if c.viewType == codeViewType {
		c.codeView.CursorDown()
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

	defaultList = list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	defaultList.SetShowHelp(false)

	// https://github.com/alecthomas/chroma/tree/master/styles
	// https://xyproto.github.io/splash/docs/
	codeView = codeview.New("friendly")

	return columnView{
		id:       mouseZone.NewPrefix(),
		focused:  focused,
		viewType: viewType,
		list:     defaultList,
		codeView: codeView,
	}
}

// Init does initial setup.
func (c *columnView) Init() tea.Cmd {
	return nil
}

// Update handles all the I/O.
func (c *columnView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		c.setSize(message.Width, message.Height)
		c.codeView.SetSize(message.Width/3, message.Height/3)

		if c.viewType == listViewType {
			c.list.SetSize(message.Width/4, message.Height/3)
		}
		if c.viewType == listViewType2 {
			c.list.SetSize(message.Width/3, message.Height/3)
		}
	}

	if c.viewType == listViewType || c.viewType == listViewType2 {
		c.list, cmd = c.list.Update(msg)
	}
	if c.viewType == codeViewType {
		c.codeView, cmd = c.codeView.Update(msg)
	}

	return c, cmd
}

func (c *columnView) View() string {
	if c.viewType == listViewType {
		return mouseZone.Mark(c.id, c.getStyle().Render(c.list.View()))
	}
	if c.viewType == listViewType2 {
		return mouseZone.Mark(c.id, c.getStyle().Render(c.list.View()))
	}
	if c.viewType == codeViewType {
		return mouseZone.Mark(c.id, c.getStyle().Render(c.codeView.View()))
	}

	return ""
}

func (c *columnView) setSize(width, height int) {
	if c.viewType == listViewType {
		c.width = width / 4
	}
	if c.viewType == listViewType2 {
		c.width = width / 3
	}
	if c.viewType == codeViewType {
		c.width = width / 3
	}
}

func (c *columnView) getStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Padding(0).Margin(2).Height(c.height).Width(c.width)

	if c.Focused() {
		s = s.Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(theme.ColorBorder))
	} else {
		s = s.Border(lipgloss.HiddenBorder())
	}

	return s
}

// SetContent sets content.
func (c *columnView) SetContent(content string, language string) {
	c.codeView.SetContent(content, language)
}
