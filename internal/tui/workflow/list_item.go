package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var (
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = theme.Styles.SelectedItem
)

type listItem struct {
	name string
	item string
}

type listItemDelegate struct{}

func (d listItemDelegate) Height() int { return 1 }

func (d listItemDelegate) Spacing() int { return 0 }

func (d listItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (i listItem) FilterValue() string { return string(i.item) }

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

	var itemName string
	if i.name != "" {
		itemName = fmt.Sprintf("%s (%s)", i.name, i.item)
	} else {
		itemName = i.item
	}

	str := itemName
	if _, err := fmt.Fprint(w, fn(str)); err != nil {
		log.Trace("Failed to write to TUI output buffer", "error", err, "item", itemName)
	}
}
