package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Execute starts the TUI app
func Execute() (string, string, error) {
	board = NewBoard()
	board.InitLists()

	p := tea.NewProgram(board)

	_, err := p.Run()
	if err != nil {
		return "", "", err
	}

	return board.component, board.stack, nil
}
