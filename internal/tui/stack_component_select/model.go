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
	help               help.Model
	loaded             bool
	columnViews        []columnView
	quit               bool
	commands           []string
	components         []string
	stacks             []string
	selectedCommand    string
	selectedComponent  string
	selectedStack      string
	componentsInStacks bool
	columnPointer      int
	commandsPointer    int
	stacksPointer      int
	componentsPointer  int
}

func (app *App) getNextViewPointer() int {
	if app.columnPointer == 2 {
		return 0
	}
	return app.columnPointer + 1
}

func (app *App) getPrevViewPointer() int {
	if app.columnPointer == 0 {
		return 2
	}
	return app.columnPointer - 1
}

func NewApp(commands []string, components []string, stacks []string) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:               h,
		columnPointer:      0,
		commands:           commands,
		components:         components,
		stacks:             stacks,
		selectedComponent:  "",
		selectedStack:      "",
		selectedCommand:    "",
		componentsInStacks: true,
		commandsPointer:    0,
		stacksPointer:      1,
		componentsPointer:  2,
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
					app.columnPointer = i
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
			app.columnPointer = app.getPrevViewPointer()
			app.columnViews[app.columnPointer].Focus()
			return app, nil
		case key.Matches(message, keys.Right):
			app.columnViews[app.columnPointer].Blur()
			app.columnPointer = app.getNextViewPointer()
			app.columnViews[app.columnPointer].Focus()
			return app, nil
		case key.Matches(message, keys.FlipStacksComponents):
			if app.componentsInStacks {
				app.componentsInStacks = false
				app.stacksPointer = 2
				app.componentsPointer = 1
			} else {
				app.componentsInStacks = true
				app.stacksPointer = 1
				app.componentsPointer = 2
			}
			return app, nil

			//if app.columnPointer == 1 {
			//	app.columnViews[1].Focus()
			//	app.columnViews[2].Blur()
			//} else if app.columnPointer == 2 {
			//	app.columnViews[1].Blur()
			//	app.columnViews[2].Focus()
			//}
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
		app.columnViews[app.commandsPointer].View(),
		app.columnViews[app.stacksPointer].View(),
		app.columnViews[app.componentsPointer].View(),
	)

	return mouseZone.Scan(lipgloss.JoinVertical(lipgloss.Left, layout, app.help.View(keys)))
}

func (app *App) InitViews(commands []string, components []string, stacks []string) {
	app.columnViews = []columnView{
		newColumn(app.commandsPointer),
		newColumn(app.stacksPointer),
		newColumn(app.componentsPointer),
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

	app.columnViews[app.commandsPointer].list.Title = "Commands"
	app.columnViews[app.commandsPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[app.commandsPointer].list.SetItems(commandItems)
	app.columnViews[app.commandsPointer].list.SetFilteringEnabled(true)
	app.columnViews[app.commandsPointer].list.SetShowFilter(true)
	app.columnViews[app.commandsPointer].list.InfiniteScrolling = true

	app.columnViews[app.stacksPointer].list.Title = "Stacks"
	app.columnViews[app.stacksPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[app.stacksPointer].list.SetItems(stackItems)
	app.columnViews[app.stacksPointer].list.SetFilteringEnabled(true)
	app.columnViews[app.stacksPointer].list.SetShowFilter(true)
	app.columnViews[app.stacksPointer].list.InfiniteScrolling = true

	app.columnViews[app.componentsPointer].list.Title = "Components"
	app.columnViews[app.componentsPointer].list.SetDelegate(listItemDelegate{})
	app.columnViews[app.componentsPointer].list.SetItems(componentItems)
	app.columnViews[app.componentsPointer].list.SetFilteringEnabled(true)
	app.columnViews[app.componentsPointer].list.SetShowFilter(true)
	app.columnViews[app.componentsPointer].list.InfiniteScrolling = true
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
