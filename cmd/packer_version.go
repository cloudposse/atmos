package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer version`.
var (
	packerVersionShort = "Show Packer version."
	packerVersionLong  = `Use the command to show Packer version.

Example usage:
  atmos packer version
`
)

// packerVersionCmd represents the `atmos packer version` command.
var packerVersionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{},
	Short:   packerVersionShort,
	Long:    packerVersionLong, RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "version", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	packerParser.RegisterFlags(packerVersionCmd)
	packerCmd.AddCommand(packerVersionCmd)
}
