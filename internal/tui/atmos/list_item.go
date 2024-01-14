package atmos

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("#10ff10"))
)

type listItem string

type listItemDelegate struct{}

func (d listItemDelegate) Height() int { return 1 }

func (d listItemDelegate) Spacing() int { return 0 }

func (d listItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (i listItem) FilterValue() string { return string(i) }

func (d listItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(listItem)
	if !ok {
		return
	}

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	str := fmt.Sprintf("%s", i)
	_, _ = fmt.Fprint(w, fn(str))
}
