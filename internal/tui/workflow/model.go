package workflow

import (
	"fmt"
	"sort"

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
	help                 help.Model
	loaded               bool
	listColumnViews      []listColumnView
	codeColumnView       codeColumnView
	quit                 bool
	workflows            map[string]schema.WorkflowConfig
	selectedWorkflowFile string
	selectedWorkflow     string
	columnPointer        int
}

func NewApp(workflows map[string]schema.WorkflowConfig) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:                 h,
		columnPointer:        0,
		selectedWorkflowFile: "",
		selectedWorkflow:     "",
		workflows:            workflows,
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
		var res tea.Model
		app.help.Width = message.Width
		res, cmd = app.codeColumnView.Update(message)
		app.codeColumnView = *res.(*codeColumnView)
		cmds = append(cmds, cmd)
		for i := 0; i < len(app.listColumnViews); i++ {
			res, cmd = app.listColumnViews[i].Update(message)
			app.listColumnViews[i] = *res.(*listColumnView)
			cmds = append(cmds, cmd)
		}
		app.loaded = true
		return app, tea.Batch(cmds...)

	case tea.MouseMsg:
		if message.Button == tea.MouseButtonWheelUp {
			app.listColumnViews[app.columnPointer].list.CursorUp()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonWheelDown {
			app.listColumnViews[app.columnPointer].list.CursorDown()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonLeft {
			for i := 0; i < len(app.listColumnViews); i++ {
				zoneInfo := mouseZone.Get(app.listColumnViews[i].id)
				if zoneInfo.InBounds(message) {
					app.listColumnViews[app.columnPointer].Blur()
					app.columnPointer = i
					app.listColumnViews[app.columnPointer].Focus()
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
			res, cmd := app.listColumnViews[app.columnPointer].Update(msg)
			app.listColumnViews[app.columnPointer] = *res.(*listColumnView)
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
			app.listColumnViews[app.columnPointer].list.CursorUp()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		case key.Matches(message, keys.Down):
			app.listColumnViews[app.columnPointer].list.CursorDown()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		case key.Matches(message, keys.Left):
			app.listColumnViews[app.columnPointer].Blur()
			app.columnPointer = app.getPrevViewPointer()
			app.listColumnViews[app.columnPointer].Focus()
			return app, nil
		case key.Matches(message, keys.Right):
			app.listColumnViews[app.columnPointer].Blur()
			app.columnPointer = app.getNextViewPointer()
			app.listColumnViews[app.columnPointer].Focus()
			return app, nil
		}
	}

	// Send all other messages to the selected child view
	res, cmd := app.listColumnViews[app.columnPointer].Update(msg)
	app.listColumnViews[app.columnPointer] = *res.(*listColumnView)
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
		app.listColumnViews[0].View(),
		app.listColumnViews[1].View(),
		app.codeColumnView.View(),
	)

	return mouseZone.Scan(lipgloss.JoinVertical(lipgloss.Left, layout, app.help.View(keys)))
}

func (app *App) GetSelectedWorkflowFile() string {
	return app.selectedWorkflowFile
}

func (app *App) GetSelectedWorkflow() string {
	return app.selectedWorkflow
}

func (app *App) ExitStatusQuit() bool {
	return app.quit
}

func (app *App) initViews(workflows map[string]schema.WorkflowConfig) {
	app.listColumnViews = []listColumnView{
		newListColumn(0),
		newListColumn(1),
	}

	app.codeColumnView = newCodeViewColumn("solarized-dark256")

	workflowFileItems := []list.Item{}
	workflowItems := []list.Item{}
	workflowFilesMapKeys := lo.Keys(workflows)
	sort.Strings(workflowFilesMapKeys)

	if len(workflowFilesMapKeys) > 0 {
		workflowFileItems = lo.Map(workflowFilesMapKeys, func(s string, _ int) list.Item {
			return listItem(s)
		})

		firstWorkflowFile := workflowFilesMapKeys[0]
		workflowItems = lo.Map(lo.Keys(workflows[firstWorkflowFile]), func(s string, _ int) list.Item {
			return listItem(s)
		})
	}

	app.listColumnViews[0].list.Title = "Workflow Manifests"
	app.listColumnViews[0].list.SetDelegate(listItemDelegate{})
	app.listColumnViews[0].list.SetItems(workflowFileItems)
	app.listColumnViews[0].list.SetFilteringEnabled(true)
	app.listColumnViews[0].list.SetShowFilter(true)
	app.listColumnViews[0].list.InfiniteScrolling = true

	app.listColumnViews[1].list.Title = "Workflows"
	app.listColumnViews[1].list.SetDelegate(listItemDelegate{})
	app.listColumnViews[1].list.SetItems(workflowItems)
	app.listColumnViews[1].list.SetFilteringEnabled(true)
	app.listColumnViews[1].list.SetShowFilter(true)
	app.listColumnViews[1].list.InfiniteScrolling = true

	app.codeColumnView.SetContent("workflows: {}", "yaml")
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

func (app *App) updateWorkflowFilesAndWorkflowsViews() {
	if app.columnPointer == 0 {
		selected := app.listColumnViews[0].list.SelectedItem()
		if selected == nil {
			return
		}
		selectedItem := fmt.Sprintf("%s", selected)
		itemStrings := lo.Keys(app.workflows[selectedItem])
		items := lo.Map(itemStrings, func(s string, _ int) list.Item {
			return listItem(s)
		})
		app.listColumnViews[1].list.ResetFilter()
		app.listColumnViews[1].list.ResetSelected()
		app.listColumnViews[1].list.SetItems(items)
	}
}

func (app *App) execute() {
	app.quit = false
	workflowFilesViewIndex := 0
	workflowsViewIndex := 1

	selectedWorkflowFile := app.listColumnViews[workflowFilesViewIndex].list.SelectedItem()
	if selectedWorkflowFile != nil {
		app.selectedWorkflowFile = fmt.Sprintf("%s", selectedWorkflowFile)
	} else {
		app.selectedWorkflowFile = ""
	}

	selectedWorkflow := app.listColumnViews[workflowsViewIndex].list.SelectedItem()
	if selectedWorkflow != nil {
		app.selectedWorkflow = fmt.Sprintf("%s", selectedWorkflow)
	} else {
		app.selectedWorkflow = ""
	}
}
