package main

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	progress progress.Model
	current  float64
}

func (m model) Init() tea.Cmd {
	// IMPORTANT: We need BOTH the progress init AND a tick command
	return tea.Batch(
		m.progress.Init(),
		tickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tickMsg time.Time

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit
	case tickMsg:
		m.current += 0.01
		if m.current > 1.0 {
			return m, tea.Quit
		}
		// KEY: SetPercent returns a Cmd that must be returned\!
		cmd := m.progress.SetPercent(m.current)
		return m, tea.Batch(tickCmd(), cmd)
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	return fmt.Sprintf("\n  %s  %.0f%%\n\n", m.progress.View(), m.current*100)
}

func main() {
	prog := progress.New(progress.WithDefaultGradient())
	if _, err := tea.NewProgram(model{progress: prog}).Run(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
