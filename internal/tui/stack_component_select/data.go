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
	if pointer == execute {
		return stacks
	}
	return pointer + 1
}

func (pointer columnPointer) getPrev() columnPointer {
	if pointer == stacks {
		return execute
	}
	return pointer - 1
}

var app *App

const (
	stacks columnPointer = iota
	components
	execute
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

func (i listItem) FilterValue() string { return (string(i)) }

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

	fmt.Fprint(w, fn(str))
}

func (app *App) InitViews() {
	app.cols = []column{
		newColumn(stacks),
		newColumn(components),
		newColumn(execute),
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

	app.cols[stacks].list.Title = "Stacks"
	app.cols[stacks].list.SetDelegate(listItemDelegate{})
	app.cols[stacks].list.SetItems(items)
	app.cols[stacks].list.SetFilteringEnabled(true)
	app.cols[stacks].list.SetShowFilter(true)

	app.cols[components].list.Title = "Components"
	app.cols[components].list.SetDelegate(listItemDelegate{})
	app.cols[components].list.SetItems(items)
	app.cols[components].list.SetFilteringEnabled(true)
	app.cols[components].list.SetShowFilter(true)

	app.cols[execute].list.Title = "Execute"
}
