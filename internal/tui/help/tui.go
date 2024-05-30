package help

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Execute starts the help TUI
func Execute(content string) (*App, error) {
	app, err := NewApp(content)
	if err != nil {
		return nil, err
	}

	p := tea.NewProgram(app, tea.WithMouseCellMotion())

	_, err = p.Run()
	if err != nil {
		return nil, err
	}

	return app, nil
}
