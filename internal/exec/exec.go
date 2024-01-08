package exec

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	tui "github.com/cloudposse/atmos/internal/tui/stack_component_select"
	"github.com/spf13/cobra"
)

func ExecuteExecCmd(cmd *cobra.Command, args []string) error {
	board := tui.NewBoard()
	board.InitLists()
	p := tea.NewProgram(board)

	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return nil
}
