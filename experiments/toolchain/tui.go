package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	spinner  spinner.Model
	progress progress.Model
	percent  float64
	quitting bool
}

func initialModel() model {
	return model{
		spinner:  spinner.New(),
		progress: progress.New(progress.WithDefaultGradient()),
		percent:  0.0,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, nil)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "p":
			if m.percent < 1.0 {
				m.percent += 0.1
			}
			return m, nil
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}
	bar := m.progress.ViewAs(m.percent)
	return lipgloss.NewStyle().Padding(1, 2).Render(
		fmt.Sprintf("%s Installing...\n%s\nPress 'p' to increment progress, 'q' to quit.", m.spinner.View(), bar),
	)
}

func runTUI() error {
	p := tea.NewProgram(initialModel())
	_, err := p.Run()
	return err
}
