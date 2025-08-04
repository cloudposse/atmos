package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer output`.
var (
	packerOutputShort = "Get an output from a Packer manifest."
	packerOutputLong  = `The command is used to get an output for a build from a Packer manifest.

Example usage:
  atmos packer output <component> -s <stack>
  atmos packer output <component> -s <stack> --query <yq-expression>
  atmos packer output <component> --stack <stack> --query <yq-expression>
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
	packerOutputCmd.PersistentFlags().String("query", "", "YQ expression to read the output from the manifest")

	packerCmd.AddCommand(packerOutputCmd)
}
