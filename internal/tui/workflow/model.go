package workflow

import (
	"sort"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	mouseZone "github.com/lrstanley/bubblezone"
	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type App struct {
	help                          help.Model
	columnViews                   []columnView
	columnPointer                 int
	selectedWorkflowFile          string
	selectedWorkflow              string
	selectedWorkflowStep          string
	workflows                     map[string]schema.WorkflowManifest
	workflowStepsViewShowWorkflow bool
	quit                          bool
	loaded                        bool
}

func NewApp(workflows map[string]schema.WorkflowManifest) *App {
	h := help.New()
	h.ShowAll = true

	app := &App{
		help:                          h,
		columnPointer:                 0,
		selectedWorkflowFile:          "",
		selectedWorkflow:              "",
		selectedWorkflowStep:          "",
		workflows:                     workflows,
		workflowStepsViewShowWorkflow: false,
	}

	app.initViews(workflows)

	return app
}

func (app *App) initViews(workflows map[string]schema.WorkflowManifest) {
	app.columnViews = []columnView{
		newColumn(0, listViewType),
		newColumn(1, listViewType),
		newColumn(2, listViewType2),
	}

	workflowFileItems := []list.Item{}
	workflowItems := []list.Item{}
	stepItems := []list.Item{}

	workflowFilesMapKeys := lo.Keys(workflows)
	sort.Strings(workflowFilesMapKeys)
	var selectedWorkflowContent string

	if len(workflowFilesMapKeys) > 0 {
		workflowFileItems = lo.Map(workflowFilesMapKeys, func(s string, _ int) list.Item {
			workflowManifest := workflows[s]
			return listItem{
				name: workflowManifest.Name,
				item: s,
			}
		})

		selectedWorkflowFileName := workflowFilesMapKeys[0]
		workflowsMapKeys := lo.Keys(workflows[selectedWorkflowFileName].Workflows)
		sort.Strings(workflowsMapKeys)

		if len(workflowsMapKeys) > 0 {
			workflowItems = lo.Map(workflowsMapKeys, func(s string, _ int) list.Item {
				return listItem{
					item: s,
				}
			})
			selectedWorkflowName := workflowsMapKeys[0]
			selectedWorkflowContent, _ = u.ConvertToYAML(workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName])

			selectedWorkflowDefinition := workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName]
			stepItems = lo.Map(selectedWorkflowDefinition.Steps, func(s schema.WorkflowStep, _ int) list.Item {
				name := s.Name
				if name == "" {
					name = s.Command
				}
				return listItem{
					name: name,
					item: s.Name,
				}
			})
		}
	}

	app.columnViews[0].list.Title = "Workflow Manifests"
	app.columnViews[0].list.SetDelegate(listItemDelegate{})
	app.columnViews[0].list.SetItems(workflowFileItems)
	app.columnViews[0].list.SetFilteringEnabled(true)
	app.columnViews[0].list.SetShowFilter(true)
	app.columnViews[0].list.InfiniteScrolling = true

	app.columnViews[1].list.Title = "Workflows"
	app.columnViews[1].list.SetDelegate(listItemDelegate{})
	app.columnViews[1].list.SetItems(workflowItems)
	app.columnViews[1].list.SetFilteringEnabled(true)
	app.columnViews[1].list.SetShowFilter(true)
	app.columnViews[1].list.InfiniteScrolling = true

	app.columnViews[2].list.Title = "Execute From Step"
	app.columnViews[2].list.SetDelegate(listItemDelegate{})
	app.columnViews[2].list.SetItems(stepItems)
	app.columnViews[2].list.SetFilteringEnabled(true)
	app.columnViews[2].list.SetShowFilter(true)
	app.columnViews[2].list.InfiniteScrolling = true
	app.columnViews[2].SetContent(selectedWorkflowContent, "yaml")
}

func (app *App) updateWorkflowFilesAndWorkflowsViews() {
	switch app.columnPointer {
	case 0:
		selectedWorkflowFile := app.columnViews[0].list.SelectedItem()
		if selectedWorkflowFile == nil {
			return
		}

		selectedWorkflowFileName := selectedWorkflowFile.(listItem).item
		workflowsMapKeys := lo.Keys(app.workflows[selectedWorkflowFileName].Workflows)
		sort.Strings(workflowsMapKeys)

		if len(workflowsMapKeys) > 0 {
			workflowItems := lo.Map(workflowsMapKeys, func(s string, _ int) list.Item {
				return listItem{
					item: s,
				}
			})

			app.columnViews[1].list.ResetFilter()
			app.columnViews[1].list.ResetSelected()
			app.columnViews[1].list.SetItems(workflowItems)

			selectedWorkflowName := workflowsMapKeys[0]
			selectedWorkflowContent, _ := u.ConvertToYAML(app.workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName])
			app.columnViews[2].SetContent(selectedWorkflowContent, "yaml")

			selectedWorkflowDefinition := app.workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName]
			stepItems := lo.Map(selectedWorkflowDefinition.Steps, func(s schema.WorkflowStep, _ int) list.Item {
				name := s.Name
				if name == "" {
					name = s.Command
				}
				return listItem{
					name: name,
					item: s.Name,
				}
			})
			app.columnViews[2].list.ResetFilter()
			app.columnViews[2].list.ResetSelected()
			app.columnViews[2].list.SetItems(stepItems)
		}
	case 1:
		selectedWorkflowFile := app.columnViews[0].list.SelectedItem()
		if selectedWorkflowFile == nil {
			return
		}

		selectedWorkflowFileName := selectedWorkflowFile.(listItem).item

		selectedWorkflow := app.columnViews[1].list.SelectedItem()
		if selectedWorkflow == nil {
			return
		}

		selectedWorkflowName := selectedWorkflow.(listItem).item
		selectedWorkflowContent, _ := u.ConvertToYAML(app.workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName])
		app.columnViews[2].SetContent(selectedWorkflowContent, "yaml")

		selectedWorkflowDefinition := app.workflows[selectedWorkflowFileName].Workflows[selectedWorkflowName]
		stepItems := lo.Map(selectedWorkflowDefinition.Steps, func(s schema.WorkflowStep, _ int) list.Item {
			name := s.Name
			if name == "" {
				name = s.Command
			}
			return listItem{
				name: name,
				item: s.Name,
			}
		})
		app.columnViews[2].list.ResetFilter()
		app.columnViews[2].list.ResetSelected()
		app.columnViews[2].list.SetItems(stepItems)
	}
}

func (app *App) flipWorkflowStepsView() {
	if app.columnViews[2].viewType == listViewType2 {
		app.columnViews[2].viewType = codeViewType
	} else {
		app.columnViews[2].viewType = listViewType2
	}
	app.workflowStepsViewShowWorkflow = !app.workflowStepsViewShowWorkflow
}

func (app *App) execute() {
	app.quit = false
	workflowFilesViewIndex := 0
	workflowsViewIndex := 1
	workflowStepViewIndex := 2

	selectedWorkflowFile := app.columnViews[workflowFilesViewIndex].list.SelectedItem()
	if selectedWorkflowFile != nil {
		app.selectedWorkflowFile = selectedWorkflowFile.(listItem).item
	} else {
		app.selectedWorkflowFile = ""
	}

	selectedWorkflow := app.columnViews[workflowsViewIndex].list.SelectedItem()
	if selectedWorkflow != nil {
		app.selectedWorkflow = selectedWorkflow.(listItem).item
	} else {
		app.selectedWorkflow = ""
	}

	selectedWorkflowStep := app.columnViews[workflowStepViewIndex].list.SelectedItem()
	if selectedWorkflowStep != nil {
		app.selectedWorkflowStep = selectedWorkflowStep.(listItem).item
	} else {
		app.selectedWorkflowStep = ""
	}
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
			app.columnViews[app.columnPointer].CursorUp()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		}
		if message.Button == tea.MouseButtonWheelDown {
			app.columnViews[app.columnPointer].CursorDown()
			app.updateWorkflowFilesAndWorkflowsViews()
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
			if app.columnViews[app.columnPointer].viewType == listViewType || app.columnViews[app.columnPointer].viewType == listViewType2 {
				res, cmd := app.columnViews[app.columnPointer].Update(msg)
				app.columnViews[app.columnPointer] = *res.(*columnView)
				if cmd == nil {
					return app, nil
				} else {
					app.quit = true
					return app, tea.Quit
				}
			}
			app.quit = true
			return app, tea.Quit
		case key.Matches(message, keys.Execute):
			app.execute()
			return app, tea.Quit
		case key.Matches(message, keys.Up):
			app.columnViews[app.columnPointer].CursorUp()
			app.updateWorkflowFilesAndWorkflowsViews()
			return app, nil
		case key.Matches(message, keys.Down):
			app.columnViews[app.columnPointer].CursorDown()
			app.updateWorkflowFilesAndWorkflowsViews()
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
		case key.Matches(message, keys.FlipWorkflowStepsView):
			app.flipWorkflowStepsView()
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

func (app *App) GetSelectedWorkflowFile() string {
	return app.selectedWorkflowFile
}

func (app *App) GetSelectedWorkflow() string {
	return app.selectedWorkflow
}

func (app *App) GetSelectedWorkflowStep() string {
	return app.selectedWorkflowStep
}

func (app *App) ExitStatusQuit() bool {
	return app.quit
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
