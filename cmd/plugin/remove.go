package plugin

import (
	"github.com/spf13/cobra"
)

func removeCommand() *cobra.Command {
	return &cobra.Command{Use: "remove <alias>", Short: "Remove a plugin declaration and its lock entry", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if atmosConfig == nil {
			return errNoProject()
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		return manager.Remove(cmd.Context(), args[0])
	}}
}
