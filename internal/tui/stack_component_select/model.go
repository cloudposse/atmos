package stack_component_select

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samber/lo"
)

type App struct {
	help              help.Model
	loaded            bool
	columnPointer     columnPointer
	colViews          []columnView
	quitting          bool
	commands          []string
	components        []string
	stacks            []string
	selectedCommand   string
	selectedComponent string
	selectedStack     string
}

type columnPointer int

const (
	commandsPointer columnPointer = iota
	stacksPointer
	componentsPointer
)

func (pointer columnPointer) getNextViewPointer() columnPointer {
	if pointer == componentsPointer {
		return commandsPointer
	}
	return pointer + 1
}

func (pointer columnPointer) getPrevViewPointer() columnPointer {
	if pointer == commandsPointer {
		return componentsPointer
	}
	return pointer - 1
}

func NewApp(commands []string, components []string, stacks []string) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:              h,
		columnPointer:     commandsPointer,
		commands:          commands,
		components:        components,
		stacks:            stacks,
		selectedComponent: "vpc",
		selectedStack:     "plat-ue2-dev",
		selectedCommand:   "",
	}

	app.InitViews(commands, components, stacks)

	return app
}

func (app *App) Init() tea.Cmd {
	return nil
}

func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		app.loaded = false
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
			app.columnPointer = app.columnPointer.getPrevViewPointer()
			app.colViews[app.columnPointer].Focus()
		case key.Matches(msg, keys.Right):
			app.colViews[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getNextViewPointer()
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

	layout := lipgloss.JoinHorizontal(
		lipgloss.Left,
		app.colViews[commandsPointer].View(),
		app.colViews[stacksPointer].View(),
		app.colViews[componentsPointer].View(),
	)

	return lipgloss.JoinVertical(lipgloss.Left, layout, app.help.View(keys))
}

func (app *App) InitViews(commands []string, components []string, stacks []string) {
	app.colViews = []columnView{
		newColumn(commandsPointer),
		newColumn(stacksPointer),
		newColumn(componentsPointer),
	}

	commandItems := lo.Map(commands, func(s string, index int) list.Item {
		return listItem(s)
	})

	stackItems := lo.Map(stacks, func(s string, index int) list.Item {
		return listItem(s)
	})

	componentItems := lo.Map(components, func(s string, index int) list.Item {
		return listItem(s)
	})

	app.colViews[commandsPointer].list.Title = "Commands"
	app.colViews[commandsPointer].list.SetDelegate(listItemDelegate{})
	app.colViews[commandsPointer].list.SetItems(commandItems)
	app.colViews[commandsPointer].list.SetFilteringEnabled(true)
	app.colViews[commandsPointer].list.SetShowFilter(true)
	app.colViews[commandsPointer].list.InfiniteScrolling = true

	app.colViews[stacksPointer].list.Title = "Stacks"
	app.colViews[stacksPointer].list.SetDelegate(listItemDelegate{})
	app.colViews[stacksPointer].list.SetItems(stackItems)
	app.colViews[stacksPointer].list.SetFilteringEnabled(true)
	app.colViews[stacksPointer].list.SetShowFilter(true)
	app.colViews[stacksPointer].list.InfiniteScrolling = true

	app.colViews[componentsPointer].list.Title = "Components"
	app.colViews[componentsPointer].list.SetDelegate(listItemDelegate{})
	app.colViews[componentsPointer].list.SetItems(componentItems)
	app.colViews[componentsPointer].list.SetFilteringEnabled(true)
	app.colViews[componentsPointer].list.SetShowFilter(true)
	app.colViews[componentsPointer].list.InfiniteScrolling = true
}
