package stack_component_select

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type columnPointer int

func (pointer columnPointer) getNext() columnPointer {
	if pointer == executePointer {
		return stacksPointer
	}
	return pointer + 1
}

func (pointer columnPointer) getPrev() columnPointer {
	if pointer == stacksPointer {
		return executePointer
	}
	return pointer - 1
}

const (
	stacksPointer columnPointer = iota
	componentsPointer
	executePointer
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("#00ff00"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
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

	str := fmt.Sprintf("%s", i)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	_, _ = fmt.Fprint(w, fn(str))
}

func (app *App) InitViews(components []string, stacks []string) {
	app.cols = []column{
		newColumn(stacksPointer),
		newColumn(componentsPointer),
		newColumn(executePointer),
	}

	items := []list.Item{
		listItem("Ramen"),
		listItem("Tomato Soup"),
		listItem("Hamburgers"),
		listItem("Cheeseburgers"),
		listItem("Currywurst"),
		listItem("Okonomiyaki"),
		listItem("Pasta"),
		listItem("Fillet Mignon"),
		listItem("Caviar"),
		listItem("Just Wine"),
	}

	app.cols[stacksPointer].list.Title = "Stacks"
	app.cols[stacksPointer].list.SetDelegate(listItemDelegate{})
	app.cols[stacksPointer].list.SetItems(items)
	app.cols[stacksPointer].list.SetFilteringEnabled(true)
	app.cols[stacksPointer].list.SetShowFilter(true)

	app.cols[componentsPointer].list.Title = "Components"
	app.cols[componentsPointer].list.SetDelegate(listItemDelegate{})
	app.cols[componentsPointer].list.SetItems(items)
	app.cols[componentsPointer].list.SetFilteringEnabled(true)
	app.cols[componentsPointer].list.SetShowFilter(true)

	app.cols[executePointer].list.Title = "Execute"
}
