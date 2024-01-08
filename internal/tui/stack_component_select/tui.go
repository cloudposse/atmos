package stack_component_select

import (
	tea "github.com/charmbracelet/bubbletea"
	e "github.com/cloudposse/atmos/internal/exec"
)

// Execute starts the TUI app
func Execute() error {
	board = NewBoard()
	board.InitLists()
	p := tea.NewProgram(board)

	if _, err := p.Run(); err != nil {
		return err
	}

	if _, err := e.ExecuteDescribeComponent("vpc", "plat-ue2-dev"); err != nil {
		return err
	}

	return nil
}
