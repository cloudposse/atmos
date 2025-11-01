// https://developer.hashicorp.com/packer/docs/commands/validate

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer validate`.
var (
	packerValidateShort = "Validate a Packer template."
	packerValidateLong  = `The command is used to validate the syntax and configuration of a Packer template.

Example usage:
  atmos packer validate <component> --stack <stack> [options]
  atmos packer validate <component> --stack <stack> --template <template> [options]
  atmos packer validate <component> -s <stack> --t <template> [options]

To see all available options, refer to https://developer.hashicorp.com/packer/docs/commands/validate
`
)

// packerValidateCmd represents the `atmos packer validate` command.
var packerValidateCmd = &cobra.Command{
	Use:     "validate",
	Aliases: []string{},
	Short:   packerValidateShort,
	Long:    packerValidateLong, RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "validate", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	packerParser.RegisterFlags(packerValidateCmd)
	packerCmd.AddCommand(packerValidateCmd)
}
