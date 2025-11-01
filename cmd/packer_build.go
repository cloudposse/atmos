// https://developer.hashicorp.com/packer/docs/commands/build

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos packer build`.
var (
	packerBuildShort = "Build a machine image from a Packer configuration."
	packerBuildLong  = `This command takes a Packer template and runs all the builds within it in order to generate a set of artifacts.

Example usage:
  atmos packer build <component> --stack <stack> [options]
  atmos packer build <component> --stack <stack> --template <template> [options]
  atmos packer build <component> -s <stack> --t <template> [options]

To see all available options, refer to https://developer.hashicorp.com/packer/docs/commands/build
`
)

// packerBuildCmd represents the `atmos packer build` command.
var packerBuildCmd = &cobra.Command{
	Use:     "build",
	Aliases: []string{},
	Short:   packerBuildShort,
	Long:    packerBuildLong, RunE: func(cmd *cobra.Command, args []string) error {
		return packerRun(cmd, "build", args)
	},
}

func init() {
	// Register Atmos flags on this subcommand
	packerParser.RegisterFlags(packerBuildCmd)
	packerCmd.AddCommand(packerBuildCmd)
}
