package pro

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// unlockCmd executes 'pro unlock' CLI command.
var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock a stack",
	Long:  `This command calls the atmos pro API and unlocks a stack`,
	Args:  cobra.NoArgs,
	RunE:  e.ExecuteProUnlockCommand,
}

func init() {
	unlockCmd.PersistentFlags().StringP("component", "c", "", "Specify the Atmos component to lock")
	unlockCmd.PersistentFlags().StringP("stack", "s", "", "Specify the Atmos stack to lock")
}
