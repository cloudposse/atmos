package help

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render

type App struct {
	viewport viewport.Model
}

func NewApp(content string) (*App, error) {
	const width = 90
	const height = 30

	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil, err
	}

	str, err := renderer.Render(content)
	if err != nil {
		return nil, err
	}

	vp.SetContent(str)

	return &App{
		viewport: vp,
	}, nil
}

func (app App) Init() tea.Cmd {
	return nil
}

func (app App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return app, tea.Quit
		default:
			var cmd tea.Cmd
			app.viewport, cmd = app.viewport.Update(msg)
			return app, cmd
		}
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			app.viewport.LineUp(1)
		}
		if msg.Button == tea.MouseButtonWheelDown {
			app.viewport.LineDown(1)
		}
		return app, nil
	default:
		return app, nil
	}
}

func (app App) View() string {
	return app.viewport.View() + app.helpView()
}

func (app App) helpView() string {
	return helpStyle("\n  ↑/↓/mouse wheel - navigate     esc/q/ctrl+c - quit\n")
}
