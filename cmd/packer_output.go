package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer output`.
var (
	packerOutputShort = "Get an output from a Packer manifest."
	packerOutputLong  = `The command is used to get an output for a build from a Packer manifest.

Example usage:
  atmos packer output <component> --stack <stack> --build al2023 --output artifact_id
  atmos packer output <component> -s <stack> --output artifact_id
`
)

// packerOutputCmd represents the `atmos packer output` command.
var packerOutputCmd = &cobra.Command{
	Use:                "output",
	Aliases:            []string{},
	Short:              packerOutputShort,
	Long:               packerOutputLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "output", args)
	},
}

func init() {
	packerOutputCmd.PersistentFlags().String("build", "", "The name of the build from which to get the output (e.g., al2023)")
	packerOutputCmd.PersistentFlags().String("output", "", "The name of the output to get (e.g., artifact_id)")

	packerCmd.AddCommand(packerOutputCmd)
}
