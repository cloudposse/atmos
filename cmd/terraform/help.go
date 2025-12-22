package terraform

import (
	"github.com/spf13/cobra"
)

// helpCmd represents the terraform help subcommand.
// This is registered explicitly to provide consistent help behavior
// without relying on implicit arg checking that could conflict with
// components named "help".
var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Show help for terraform command",
	Long:  `Display detailed help information for the terraform command and its subcommands.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Parent().Help()
	},
}

func init() {
	terraformCmd.AddCommand(helpCmd)
}
