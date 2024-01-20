package workflow

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	mouseZone "github.com/lrstanley/bubblezone"
	"github.com/samber/lo"
)

type App struct {
	help                     help.Model
	loaded                   bool
	columnViews              []columnView
	quit                     bool
	workflows                map[string]schema.WorkflowConfig
	selectedWorkflowFile     string
	selectedWorkflow         string
	workflowsInWorkflowFiles bool
	columnPointer            int
}

func NewApp(workflows map[string]schema.WorkflowConfig) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:                     h,
		columnPointer:            0,
		selectedWorkflowFile:     "",
		selectedWorkflow:         "",
		workflowsInWorkflowFiles: true,
		workflows:                workflows,
	}

	app.initViews(workflows)

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
			app.updateWorkflowsAndWorkflowFilesViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonWheelDown {
			app.columnViews[app.columnPointer].list.CursorDown()
			app.updateWorkflowsAndWorkflowFilesViews()
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
			app.updateWorkflowsAndWorkflowFilesViews()
			return app, nil
		case key.Matches(message, keys.Down):
			app.columnViews[app.columnPointer].list.CursorDown()
			app.updateWorkflowsAndWorkflowFilesViews()
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
			app.flipWorkflowsAndWorkflowFilesViews()
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

func (app *App) GetSelectedWorkflow() string {
	return app.selectedWorkflow
}

func (app *App) GetSelectedWorkflowFile() string {
	return app.selectedWorkflowFile
}

func (app *App) ExitStatusQuit() bool {
	return app.quit
}

func (app *App) initViews(workflows map[string]schema.WorkflowConfig) {
	app.columnViews = []columnView{
		newColumn(0),
		newColumn(1),
		newColumn(2),
	}

	//commandItems := lo.Map(commands, func(s string, _ int) list.Item {
	//	return listItem(s)
	//})
	//
	//stackItems := []list.Item{}
	//componentItems := []list.Item{}
	//stacksComponentsMapKeys := lo.Keys(stacksComponentsMap)
	//sort.Strings(stacksComponentsMapKeys)
	//
	//if len(stacksComponentsMapKeys) > 0 {
	//	stackItems = lo.Map(stacksComponentsMapKeys, func(s string, _ int) list.Item {
	//		return listItem(s)
	//	})
	//
	//	firstStack := stacksComponentsMapKeys[0]
	//	componentItems = lo.Map(stacksComponentsMap[firstStack], func(s string, _ int) list.Item {
	//		return listItem(s)
	//	})
	//}

	app.columnViews[0].list.Title = "Files"
	app.columnViews[0].list.SetDelegate(listItemDelegate{})
	//app.columnViews[0].list.SetItems(commandItems)
	app.columnViews[0].list.SetFilteringEnabled(true)
	app.columnViews[0].list.SetShowFilter(true)
	app.columnViews[0].list.InfiniteScrolling = true

	app.columnViews[1].list.Title = "Workflows"
	app.columnViews[1].list.SetDelegate(listItemDelegate{})
	//app.columnViews[1].list.SetItems(stackItems)
	app.columnViews[1].list.SetFilteringEnabled(true)
	app.columnViews[1].list.SetShowFilter(true)
	app.columnViews[1].list.InfiniteScrolling = true

	app.columnViews[2].list.Title = "Workflow"
	app.columnViews[2].list.SetDelegate(listItemDelegate{})
	//app.columnViews[2].list.SetItems(componentItems)
	app.columnViews[2].list.SetFilteringEnabled(true)
	app.columnViews[2].list.SetShowFilter(true)
	app.columnViews[2].list.InfiniteScrolling = true
}

func (app *App) getNextViewPointer() int {
	if app.columnPointer == 1 {
		return 0
	}
	return app.columnPointer + 1
}

func (app *App) getPrevViewPointer() int {
	if app.columnPointer == 0 {
		return 1
	}
	return app.columnPointer - 1
}

func (app *App) updateWorkflowsAndWorkflowFilesViews() {
	if app.columnPointer == 1 {
		selected := app.columnViews[1].list.SelectedItem()
		if selected == nil {
			return
		}
		//selectedItem := fmt.Sprintf("%s", selected)
		var itemStrings []string
		//if app.workflowsInWorkflowFiles {
		//	itemStrings = app.stacksComponentsMap[selectedItem]
		//} else {
		//	itemStrings = app.componentsStacksMap[selectedItem]
		//}
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
	var workflowsViewIndex int
	var workflowFilesViewIndex int

	if app.workflowsInWorkflowFiles {
		workflowFilesViewIndex = 0
		workflowsViewIndex = 1
	} else {
		workflowFilesViewIndex = 1
		workflowsViewIndex = 0
	}

	selectedWorkflowFile := app.columnViews[workflowFilesViewIndex].list.SelectedItem()
	if selectedWorkflowFile != nil {
		app.selectedWorkflowFile = fmt.Sprintf("%s", selectedWorkflowFile)
	} else {
		app.selectedWorkflowFile = ""
	}

	selectedWorkflow := app.columnViews[workflowsViewIndex].list.SelectedItem()
	if selectedWorkflow != nil {
		app.selectedWorkflow = fmt.Sprintf("%s", selectedWorkflow)
	} else {
		app.selectedWorkflow = ""
	}
}

func (app *App) flipWorkflowsAndWorkflowFilesViews() {
	app.workflowsInWorkflowFiles = !app.workflowsInWorkflowFiles
	app.columnViews[0].list.ResetFilter()
	app.columnViews[0].list.ResetSelected()
	app.columnViews[1].list.ResetFilter()
	app.columnViews[1].list.ResetSelected()

	// Keep the focused view at the same position
	if app.columnViews[0].Focused() {
		app.columnViews[0].Blur()
		app.columnViews[1].Focus()
	} else if app.columnViews[1].Focused() {
		app.columnViews[1].Blur()
		app.columnViews[0].Focus()
	}

	// Swap workflowsFiles/workflows
	// The view will be updated by the framework
	i := app.columnViews[0]
	app.columnViews[0] = app.columnViews[1]
	app.columnViews[1] = i
}
