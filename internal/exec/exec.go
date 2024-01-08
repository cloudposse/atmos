package exec

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	board := tui.NewBoard()
	board.InitLists()
	p := tea.NewProgram(board)

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
