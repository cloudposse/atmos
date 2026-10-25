package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
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
	Use:                "version",
	Aliases:            []string{},
	Short:              packerVersionShort,
	Long:               packerVersionLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "version", args)
	},
}

func init() {
	packerCmd.AddCommand(packerVersionCmd)
	internal.RegisterCommandMetadata(
		packerVersionCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/packer/version"),
	)
}
