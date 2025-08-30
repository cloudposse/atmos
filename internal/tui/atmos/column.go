package atmos

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	mouseZone "github.com/lrstanley/bubblezone"
)

type columnView struct {
	id      string
	focused bool
	list    list.Model
	height  int
	width   int
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

func newColumn(columnPointer int) columnView {
	var focused bool
	if columnPointer == 0 {
		focused = true
	}

	defaultList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	defaultList.SetShowHelp(false)

	return columnView{
		id:      mouseZone.NewPrefix(),
		focused: focused,
		list:    defaultList,
	}
}

// Init does initial setup.
func (c *columnView) Init() tea.Cmd {
	return nil
}

// Update handles all the I/O.
func (c *columnView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if message, ok := msg.(tea.WindowSizeMsg); ok {
		c.setSize(message.Width, message.Height)
		c.list.SetSize(message.Width/4, message.Height/3)
	}
	c.list, cmd = c.list.Update(msg)
	return c, cmd
}

func (c *columnView) View() string {
	return mouseZone.Mark(c.id, c.getStyle().Render(c.list.View()))
}

func (c *columnView) setSize(width, height int) {
	c.width = width / 4
}

// applyColumnBorder applies the appropriate border style based on focus state.
func applyColumnBorder(s *lipgloss.Style, focused bool, styles *theme.StyleSet) lipgloss.Style {
	if styles == nil {
		// Fallback if theme isn't available
		if focused {
			return s.Border(lipgloss.RoundedBorder())
		}
		return s.Border(lipgloss.HiddenBorder())
	}

	// Apply theme-based styling
	if focused {
		return styles.TUI.BorderFocused.
			Padding(1, 2).
			Height(s.GetHeight()).
			Width(s.GetWidth())
	}
	return styles.TUI.BorderUnfocused.
		Padding(1, 2).
		Height(s.GetHeight()).
		Width(s.GetWidth())
}

func (c *columnView) getStyle() lipgloss.Style {
	baseStyle := lipgloss.NewStyle().Padding(1, 2).Height(c.height).Width(c.width)
	styles := theme.GetCurrentStyles()
	return applyColumnBorder(&baseStyle, c.Focused(), styles)
}
