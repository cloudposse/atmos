package stack_component_select

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type columnView struct {
	focused       bool
	columnPointer columnPointer
	list          list.Model
	height        int
	width         int
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

func newColumn(columnPointer columnPointer) columnView {
	var focused bool
	if columnPointer == commandsPointer {
		focused = true
	}

	defaultList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	defaultList.SetShowHelp(false)
	return columnView{focused: focused, columnPointer: columnPointer, list: defaultList}
}

// Init does initial setup
func (c *columnView) Init() tea.Cmd {
	return nil
}

// Update handles all the I/O
func (c *columnView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.setSize(msg.Width, msg.Height)
		c.list.SetSize(msg.Width/4, msg.Height/3)
	}
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *columnView) View() string {
	return c.getStyle().Render(c.list.View())
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
