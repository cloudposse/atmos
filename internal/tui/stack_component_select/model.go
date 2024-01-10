package stack_component_select

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	mouseZone "github.com/lrstanley/bubblezone"
	"github.com/samber/lo"
)

type App struct {
	help              help.Model
	loaded            bool
	columnPointer     columnPointer
	columnViews       []columnView
	quit              bool
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
		selectedComponent: "",
		selectedStack:     "",
		selectedCommand:   "",
	}

	app.InitViews(commands, components, stacks)

	return app
}

func (app *App) Init() tea.Cmd {
	return nil
}

func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		app.loaded = false
		var cmd tea.Cmd
		var cmds []tea.Cmd
		app.help.Width = message.Width
		for i := 0; i < len(app.columnViews); i++ {
			var res tea.Model
			res, cmd = app.columnViews[i].Update(message)
			app.columnViews[i] = *res.(*columnView)
			cmds = append(cmds, cmd)
		}
		app.loaded = true
		return app, tea.Batch(cmds...)
	case tea.MouseMsg:
		if message.Button == tea.MouseButtonWheelUp {
			app.columnViews[app.columnPointer].list.CursorUp()
			return app, nil
		}
		if message.Button == tea.MouseButtonWheelDown {
			app.columnViews[app.columnPointer].list.CursorDown()
			return app, nil
		}
		if message.Button == tea.MouseButtonLeft {
			for i := 0; i < len(app.columnViews); i++ {
				if mouseZone.Get(app.columnViews[i].list.Title).InBounds(message) {
					app.columnViews[app.columnPointer].Blur()
					app.columnPointer = columnPointer(i)
					app.columnViews[app.columnPointer].Focus()
					break
				}
			}
		}
	case tea.KeyMsg:
		switch {
		case key.Matches(message, keys.Quit):
			app.quit = true
			return app, tea.Quit
		case key.Matches(message, keys.Escape):
			res, _ := app.columnViews[app.columnPointer].Update(msg)
			app.columnViews[app.columnPointer] = *res.(*columnView)
			return app, nil
		case key.Matches(message, keys.Execute):
			app.quit = false
			return app, tea.Quit
		case key.Matches(message, keys.Left):
			app.columnViews[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getPrevViewPointer()
			app.columnViews[app.columnPointer].Focus()
		case key.Matches(message, keys.Right):
			app.columnViews[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getNextViewPointer()
			app.columnViews[app.columnPointer].Focus()
		}
	}

	res, cmd := app.columnViews[app.columnPointer].Update(msg)

	if _, ok := res.(*columnView); ok {
		app.columnViews[app.columnPointer] = *res.(*columnView)
	} else {
		return res, cmd
	}

	return app, cmd
}

func (app *App) View() string {
	if app.quit {
		return ""
	}

	if !app.loaded {
		return "loading..."
	}

	layout := lipgloss.JoinHorizontal(
		lipgloss.Left,
		app.columnViews[commandsPointer].View(),
		app.columnViews[stacksPointer].View(),
		app.columnViews[componentsPointer].View(),
	)

	return mouseZone.Scan(lipgloss.JoinVertical(lipgloss.Left, layout, app.help.View(keys)))
}

func (app *App) InitViews(commands []string, components []string, stacks []string) {
	app.columnViews = []columnView{
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

	app.columnViews[commandsPointer].list.Title = "Commands"
	app.columnViews[commandsPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[commandsPointer].list.SetItems(commandItems)
	app.columnViews[commandsPointer].list.SetFilteringEnabled(true)
	app.columnViews[commandsPointer].list.SetShowFilter(true)
	app.columnViews[commandsPointer].list.InfiniteScrolling = true

	app.columnViews[stacksPointer].list.Title = "Stacks"
	app.columnViews[stacksPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[stacksPointer].list.SetItems(stackItems)
	app.columnViews[stacksPointer].list.SetFilteringEnabled(true)
	app.columnViews[stacksPointer].list.SetShowFilter(true)
	app.columnViews[stacksPointer].list.InfiniteScrolling = true

	app.columnViews[componentsPointer].list.Title = "Components"
	app.columnViews[componentsPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[componentsPointer].list.SetItems(componentItems)
	app.columnViews[componentsPointer].list.SetFilteringEnabled(true)
	app.columnViews[componentsPointer].list.SetShowFilter(true)
	app.columnViews[componentsPointer].list.InfiniteScrolling = true
}

func (app *App) GetSelectedCommand() string {
	return app.selectedCommand
}

func (app *App) GetSelectedStack() string {
	return app.selectedStack
}

func (app *App) GetSelectedComponent() string {
	return app.selectedComponent
}

func (app *App) ExitStatusQuit() bool {
	return app.quit
}
