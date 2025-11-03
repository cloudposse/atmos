// https://developer.hashicorp.com/packer/docs/commands/inspect

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer inspect`.
var (
	packerInspectShort = "Inspect a Packer configuration."
	packerInspectLong  = `The command takes a Packer template and outputs the various components the template defines.

Example usage:
  atmos packer inspect <component> --stack <stack>
  atmos packer inspect <component> --stack <stack> --template <template>
  atmos packer inspect <component> -s <stack> --t <template>
`
)

// packerInspectCmd represents the `atmos packer inspect` command.
var packerInspectCmd = &cobra.Command{
	Use:     "inspect",
	Aliases: []string{},
	Short:   packerInspectShort,
	Long:    packerInspectLong, RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "inspect", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	// Flags are inherited from parent packerCmd as persistent flags
	packerCmd.AddCommand(packerInspectCmd)
}
