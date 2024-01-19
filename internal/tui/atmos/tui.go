package atmos

import (
	tea "github.com/charmbracelet/bubbletea"
	mouseZone "github.com/lrstanley/bubblezone"
)

// Execute starts the TUI app and returns the selected items from the views
func Execute(commands []string, stacksComponentsMap map[string][]string, componentsStacksMap map[string][]string) (*App, error) {
	mouseZone.NewGlobal()
	mouseZone.SetEnabled(true)

	app := NewApp(commands, stacksComponentsMap, componentsStacksMap)
	p := tea.NewProgram(app, tea.WithMouseCellMotion())

	_, err := p.Run()
	if err != nil {
		return nil, err
	}

	return app, nil
}
