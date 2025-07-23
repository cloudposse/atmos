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
  atmos packer inspect <component> <packer-template> --stack <stack>
  atmos packer inspect <component> <packer-template> -s <stack>
`
)

// packerInspectCmd represents the `atmos packer inspect` command.
var packerInspectCmd = &cobra.Command{
	Use:                "inspect",
	Aliases:            []string{},
	Short:              packerInspectShort,
	Long:               packerInspectLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "inspect", args)
	},
}

func init() {
	packerCmd.AddCommand(packerInspectCmd)
}
