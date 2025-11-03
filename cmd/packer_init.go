// https://developer.hashicorp.com/packer/docs/commands/init

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer init`.
var (
	packerInitShort = "Initialize Packer according to an HCL template configuration."
	packerInitLong  = `Use the command to download and install plugins according to the required_plugins block in Packer templates written in HCL.

Example usage:
  atmos packer init <component> --stack <stack> [options]
  atmos packer init <component> --stack <stack> --template <template> [options]
  atmos packer init <component> -s <stack> --t <template> [options]

To see all available options, refer to https://developer.hashicorp.com/packer/docs/commands/init
`
)

// packerInitCmd represents the `atmos packer init` command.
var packerInitCmd = &cobra.Command{
	Use:     "init",
	Aliases: []string{},
	Short:   packerInitShort,
	Long:    packerInitLong, RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "init", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	// Flags are inherited from parent packerCmd as persistent flags
	packerCmd.AddCommand(packerInitCmd)
}
