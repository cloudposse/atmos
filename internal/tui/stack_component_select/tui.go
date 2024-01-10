package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
	mouseZone "github.com/lrstanley/bubblezone"
)

// Execute starts the TUI app and retursn the selected items from the views
func Execute(commands []string, components []string, stacks []string) (string, string, string, error) {
	mouseZone.NewGlobal()
	app := NewApp(commands, components, stacks)
	p := tea.NewProgram(app, tea.WithMouseCellMotion())

	_, err := p.Run()
	if err != nil {
		return "", "", "", err
	}

	return app.selectedCommand, app.selectedComponent, app.selectedStack, nil
}
