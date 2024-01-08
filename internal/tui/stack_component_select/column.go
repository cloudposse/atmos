package stack_component_select

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const APPEND = -1

const margin = 4

type column struct {
	focus         bool
	columnPointer columnPointer
	list          list.Model
	height        int
	width         int
}

func (c *column) Focus() {
	c.focus = true
}

func (c *column) Blur() {
	c.focus = false
}

func (c *column) Focused() bool {
	return c.focus
}

func newColumn(columnPointer columnPointer) column {
	var focus bool
	if columnPointer == stacks {
		focus = true
	}

	defaultList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	defaultList.SetShowHelp(false)
	return column{focus: focus, columnPointer: columnPointer, list: defaultList}
}

// Init does initial setup for the column.
func (c *column) Init() tea.Cmd {
	return nil
}

// Update handles all the I/O for columns
func (c *column) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.setSize(msg.Width, msg.Height)
		c.list.SetSize(msg.Width/margin, msg.Height/2)
	}
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *column) View() string {
	return c.getStyle().Render(c.list.View())
}

func (c *column) DeleteCurrent() tea.Cmd {
	if len(c.list.VisibleItems()) > 0 {
		c.list.RemoveItem(c.list.Index())
	}

	var cmd tea.Cmd
	c.list, cmd = c.list.Update(nil)
	return cmd
}

func (c *column) setSize(width, height int) {
	c.width = width / margin
}

func (c *column) getStyle() lipgloss.Style {
	if c.Focused() {
		return lipgloss.NewStyle().
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Height(c.height).
			Width(c.width)
	}
	return lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.HiddenBorder()).
		Height(c.height).
		Width(c.width)
}
