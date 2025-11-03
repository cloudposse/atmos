package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer output`.
var (
	packerOutputShort = "Get an output from a Packer manifest."
	packerOutputLong  = `The command is used to get an output from a Packer manifest.

Example usage:
  atmos packer output <component> -s <stack>
  atmos packer output <component> -s <stack> --q <yq-expression>
  atmos packer output <component> --stack <stack> --query <yq-expression>
  atmos packer output <component> --stack <stack> --query '.builds[0]'
  atmos packer output <component> --stack <stack> --query '.builds[0].artifact_id'
  atmos packer output <component> --stack <stack> --query '.builds[0].artifact_id | split(":")[1]'
`
)

// packerOutputCmd represents the `atmos packer output` command.
var packerOutputCmd = &cobra.Command{
	Use:     "output",
	Aliases: []string{},
	Short:   packerOutputShort,
	Long:    packerOutputLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "output", args)
	},
}

func init() {
	// Flags are inherited from parent packerCmd as persistent flags
	packerCmd.AddCommand(packerOutputCmd)
}
