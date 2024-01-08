package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Execute starts the TUI app
func Execute() (string, string, error) {
	board = NewBoard()
	board.InitLists()

	p := tea.NewProgram(board)

	if _, err := p.Run(); err != nil {
		return "", "", err
	}

	return "vpc", "plat-ue2-dev", nil
}
