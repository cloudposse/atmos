package stack_component_select

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type App struct {
	help              help.Model
	loaded            bool
	columnPointer     columnPointer
	colViews          []columnView
	quitting          bool
	components        []string
	stacks            []string
	selectedComponent string
	selectedStack     string
}

type columnPointer int

const (
	stacksPointer columnPointer = iota
	componentsPointer
	executePointer
)

func (pointer columnPointer) getNextView() columnPointer {
	if pointer == executePointer {
		return stacksPointer
	}
	return pointer + 1
}

func (pointer columnPointer) getPrevView() columnPointer {
	if pointer == stacksPointer {
		return executePointer
	}
	return pointer - 1
}

func NewApp(components []string, stacks []string) *App {
	h := help.New()
	h.ShowAll = true

	return &App{
		help:              h,
		columnPointer:     stacksPointer,
		components:        components,
		stacks:            stacks,
		selectedComponent: "vpc",
		selectedStack:     "plat-ue2-dev",
	}
}

func (app *App) Init() tea.Cmd {
	return nil
}

func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		var cmds []tea.Cmd
		app.help.Width = msg.Width
		for i := 0; i < len(app.colViews); i++ {
			var res tea.Model
			res, cmd = app.colViews[i].Update(msg)
			app.colViews[i] = *res.(*columnView)
			cmds = append(cmds, cmd)
		}
		app.loaded = true
		return app, tea.Batch(cmds...)
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			app.quitting = true
			return app, tea.Quit
		case key.Matches(msg, keys.Left):
			app.colViews[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getPrevView()
			app.colViews[app.columnPointer].Focus()
		case key.Matches(msg, keys.Right):
			app.colViews[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getNextView()
			app.colViews[app.columnPointer].Focus()
		}
	}

	res, cmd := app.colViews[app.columnPointer].Update(msg)
	if _, ok := res.(*columnView); ok {
		app.colViews[app.columnPointer] = *res.(*columnView)
	} else {
		return res, cmd
	}

	return app, cmd
}

func (app *App) View() string {
	if app.quitting {
		return ""
	}
	if !app.loaded {
		return "loading..."
	}
	board := lipgloss.JoinHorizontal(
		lipgloss.Left,
		app.colViews[stacksPointer].View(),
		app.colViews[componentsPointer].View(),
		app.colViews[executePointer].View(),
	)
	return lipgloss.JoinVertical(lipgloss.Left, board, app.help.View(keys))
}

func (app *App) InitViews(components []string, stacks []string) {
	app.colViews = []columnView{
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

	app.colViews[stacksPointer].list.Title = "Stacks"
	app.colViews[stacksPointer].list.SetDelegate(listItemDelegate{})
	app.colViews[stacksPointer].list.SetItems(items)
	app.colViews[stacksPointer].list.SetFilteringEnabled(true)
	app.colViews[stacksPointer].list.SetShowFilter(true)

	app.colViews[componentsPointer].list.Title = "Components"
	app.colViews[componentsPointer].list.SetDelegate(listItemDelegate{})
	app.colViews[componentsPointer].list.SetItems(items)
	app.colViews[componentsPointer].list.SetFilteringEnabled(true)
	app.colViews[componentsPointer].list.SetShowFilter(true)

	app.colViews[executePointer].list.Title = "Execute"
}
