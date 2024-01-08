package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Execute starts the TUI app
func Execute(components []string, stacks []string) (string, string, error) {
	app := NewApp(components, stacks)
	app.InitViews(components, stacks)

	p := tea.NewProgram(app)

	_, err := p.Run()
	if err != nil {
		return "", "", err
	}

	return app.selectedComponent, app.selectedStack, nil
}
