package atmos

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	mouseZone "github.com/lrstanley/bubblezone"
	"github.com/samber/lo"
)

type App struct {
	help                help.Model
	loaded              bool
	columnViews         []columnView
	quit                bool
	commands            []string
	stacksComponentsMap map[string][]string
	componentsStacksMap map[string][]string
	selectedCommand     string
	selectedComponent   string
	selectedStack       string
	componentsInStacks  bool
	columnPointer       int
}

func NewApp(commands []string, stacksComponentsMap map[string][]string, componentsStacksMap map[string][]string) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:                h,
		columnPointer:       0,
		commands:            commands,
		stacksComponentsMap: stacksComponentsMap,
		componentsStacksMap: componentsStacksMap,
		selectedComponent:   "",
		selectedStack:       "",
		selectedCommand:     "",
		componentsInStacks:  true,
	}

	app.initViews(commands, stacksComponentsMap)

	return app
}

func (app *App) Init() tea.Cmd {
	return nil
}

func (app *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Process messages relevant to the parent view
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
			app.updateStackAndComponentViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonWheelDown {
			app.columnViews[app.columnPointer].list.CursorDown()
			app.updateStackAndComponentViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonLeft {
			for i := 0; i < len(app.columnViews); i++ {
				zoneInfo := mouseZone.Get(app.columnViews[i].id)
				if zoneInfo.InBounds(message) {
					app.columnViews[app.columnPointer].Blur()
					app.columnPointer = i
					app.columnViews[app.columnPointer].Focus()
					break
				}
			}
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(message, keys.CtrlC):
			app.quit = true
			return app, tea.Quit
		case key.Matches(message, keys.Escape):
			res, cmd := app.columnViews[app.columnPointer].Update(msg)
			app.columnViews[app.columnPointer] = *res.(*columnView)
			if cmd == nil {
				return app, nil
			} else {
				app.quit = true
				return app, tea.Quit
			}
		case key.Matches(message, keys.Execute):
			app.execute()
			return app, tea.Quit
		case key.Matches(message, keys.Up):
			app.columnViews[app.columnPointer].list.CursorUp()
			app.updateStackAndComponentViews()
			return app, nil
		case key.Matches(message, keys.Down):
			app.columnViews[app.columnPointer].list.CursorDown()
			app.updateStackAndComponentViews()
			return app, nil
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
			// Flip the stacks and components views
			app.flipStackAndComponentViews()
			return app, nil
		}
	}

	// Send all other messages to the selected child view
	res, cmd := app.columnViews[app.columnPointer].Update(msg)
	app.columnViews[app.columnPointer] = *res.(*columnView)
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
		app.columnViews[0].View(),
		app.columnViews[1].View(),
		app.columnViews[2].View(),
	)

	return mouseZone.Scan(lipgloss.JoinVertical(lipgloss.Left, layout, app.help.View(keys)))
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

func (app *App) initViews(commands []string, stacksComponentsMap map[string][]string) {
	app.columnViews = []columnView{
		newColumn(0),
		newColumn(1),
		newColumn(2),
	}

	commandItems := lo.Map(commands, func(s string, _ int) list.Item {
		return listItem(s)
	})

	stackItems := []list.Item{}
	componentItems := []list.Item{}
	stacksComponentsMapKeys := lo.Keys(stacksComponentsMap)
	sort.Strings(stacksComponentsMapKeys)

	if len(stacksComponentsMapKeys) > 0 {
		stackItems = lo.Map(stacksComponentsMapKeys, func(s string, _ int) list.Item {
			return listItem(s)
		})

		firstStack := stacksComponentsMapKeys[0]
		componentItems = lo.Map(stacksComponentsMap[firstStack], func(s string, _ int) list.Item {
			return listItem(s)
		})
	}

	app.columnViews[0].list.Title = "Commands"
	app.columnViews[0].list.SetDelegate(listItemDelegate{})
	app.columnViews[0].list.SetItems(commandItems)
	app.columnViews[0].list.SetFilteringEnabled(true)
	app.columnViews[0].list.SetShowFilter(true)
	app.columnViews[0].list.InfiniteScrolling = true

	app.columnViews[1].list.Title = "Stacks"
	app.columnViews[1].list.SetDelegate(listItemDelegate{})
	app.columnViews[1].list.SetItems(stackItems)
	app.columnViews[1].list.SetFilteringEnabled(true)
	app.columnViews[1].list.SetShowFilter(true)
	app.columnViews[1].list.InfiniteScrolling = true

	app.columnViews[2].list.Title = "Components"
	app.columnViews[2].list.SetDelegate(listItemDelegate{})
	app.columnViews[2].list.SetItems(componentItems)
	app.columnViews[2].list.SetFilteringEnabled(true)
	app.columnViews[2].list.SetShowFilter(true)
	app.columnViews[2].list.InfiniteScrolling = true
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

func (app *App) updateStackAndComponentViews() {
	if app.columnPointer == 1 {
		selected := app.columnViews[1].list.SelectedItem()
		if selected == nil {
			return
		}
		selectedItem := fmt.Sprintf("%s", selected)
		var itemStrings []string
		if app.componentsInStacks {
			itemStrings = app.stacksComponentsMap[selectedItem]
		} else {
			itemStrings = app.componentsStacksMap[selectedItem]
		}
		items := lo.Map(itemStrings, func(s string, _ int) list.Item {
			return listItem(s)
		})
		app.columnViews[2].list.ResetFilter()
		app.columnViews[2].list.ResetSelected()
		app.columnViews[2].list.SetItems(items)
	}
}

func (app *App) execute() {
	app.quit = false
	commandsViewIndex := 0
	var componentsViewIndex int
	var stacksViewIndex int

	selectedCommand := app.columnViews[commandsViewIndex].list.SelectedItem()
	if selectedCommand != nil {
		app.selectedCommand = fmt.Sprintf("%s", selectedCommand)
	} else {
		app.selectedCommand = ""
	}

	if app.componentsInStacks {
		stacksViewIndex = 1
		componentsViewIndex = 2
	} else {
		stacksViewIndex = 2
		componentsViewIndex = 1
	}

	selectedComponent := app.columnViews[componentsViewIndex].list.SelectedItem()
	if selectedComponent != nil {
		app.selectedComponent = fmt.Sprintf("%s", selectedComponent)
	} else {
		app.selectedComponent = ""
	}

	selectedStack := app.columnViews[stacksViewIndex].list.SelectedItem()
	if selectedStack != nil {
		app.selectedStack = fmt.Sprintf("%s", selectedStack)
	} else {
		app.selectedStack = ""
	}
}

func (app *App) flipStackAndComponentViews() {
	app.componentsInStacks = !app.componentsInStacks
	app.columnViews[1].list.ResetFilter()
	app.columnViews[1].list.ResetSelected()
	app.columnViews[2].list.ResetFilter()
	app.columnViews[2].list.ResetSelected()

	// Keep the focused view at the same position
	if app.columnViews[1].Focused() {
		app.columnViews[1].Blur()
		app.columnViews[2].Focus()
	} else if app.columnViews[2].Focused() {
		app.columnViews[2].Blur()
		app.columnViews[1].Focus()
	}

	// Swap stacks/components
	// The view will be updated by the framework
	i := app.columnViews[1]
	app.columnViews[1] = app.columnViews[2]
	app.columnViews[2] = i

	// Reset the lists
	if app.componentsInStacks {
		stacksComponentsMapKeys := lo.Keys(app.stacksComponentsMap)
		sort.Strings(stacksComponentsMapKeys)

		if len(stacksComponentsMapKeys) > 0 {
			stackItems := lo.Map(stacksComponentsMapKeys, func(s string, _ int) list.Item {
				return listItem(s)
			})
			firstStack := stacksComponentsMapKeys[0]
			componentItems := lo.Map(app.stacksComponentsMap[firstStack], func(s string, _ int) list.Item {
				return listItem(s)
			})
			app.columnViews[1].list.SetItems(stackItems)
			app.columnViews[2].list.SetItems(componentItems)
		}
	} else {
		componentsStacksMapKeys := lo.Keys(app.componentsStacksMap)
		sort.Strings(componentsStacksMapKeys)

		if len(componentsStacksMapKeys) > 0 {
			componentItems := lo.Map(componentsStacksMapKeys, func(s string, _ int) list.Item {
				return listItem(s)
			})
			firstComponent := componentsStacksMapKeys[0]
			stackItems := lo.Map(app.componentsStacksMap[firstComponent], func(s string, _ int) list.Item {
				return listItem(s)
			})
			app.columnViews[1].list.SetItems(componentItems)
			app.columnViews[2].list.SetItems(stackItems)
		}
	}
}
