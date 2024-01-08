package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Execute starts the TUI app
func Execute() (string, string, error) {
	app = NewApp()
	app.InitViews()

	p := tea.NewProgram(app)

	_, err := p.Run()
	if err != nil {
		return "", "", err
	}

	return app.component, app.stack, nil
}
