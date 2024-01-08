package stack_component_select

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type App struct {
	help          help.Model
	loaded        bool
	columnPointer columnPointer
	cols          []column
	quitting      bool
	component     string
	stack         string
}

func NewApp() *App {
	h := help.New()
	h.ShowAll = true

	return &App{
		help:          h,
		columnPointer: stacks,
		component:     "vpc",
		stack:         "plat-ue2-dev",
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
		app.help.Width = msg.Width - margin
		for i := 0; i < len(app.cols); i++ {
			var res tea.Model
			res, cmd = app.cols[i].Update(msg)
			app.cols[i] = *res.(*column)
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
			app.cols[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getPrev()
			app.cols[app.columnPointer].Focus()
		case key.Matches(msg, keys.Right):
			app.cols[app.columnPointer].Blur()
			app.columnPointer = app.columnPointer.getNext()
			app.cols[app.columnPointer].Focus()
		}
	}

	res, cmd := app.cols[app.columnPointer].Update(msg)
	if _, ok := res.(*column); ok {
		app.cols[app.columnPointer] = *res.(*column)
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
		app.cols[stacks].View(),
		app.cols[components].View(),
		app.cols[execute].View(),
	)
	return lipgloss.JoinVertical(lipgloss.Left, board, app.help.View(keys))
}
