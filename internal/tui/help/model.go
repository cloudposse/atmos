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
	const width = 100
	const height = 40

	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		PaddingRight(2)

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
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

func (e App) Init() tea.Cmd {
	return nil
}

func (e App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return e, tea.Quit
		default:
			var cmd tea.Cmd
			e.viewport, cmd = e.viewport.Update(msg)
			return e, cmd
		}
	default:
		return e, nil
	}
}

func (e App) View() string {
	return e.viewport.View() + e.helpView()
}

func (e App) helpView() string {
	return helpStyle("\n  ↑/↓ navigate   esc/q/ctrl+c quit\n")
}
