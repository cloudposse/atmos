package cmd

import (
	"github.com/spf13/cobra"
)

// terraformGenerateCmd generates configurations for terraform components
var terraformGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Terraform configuration files for Atmos components and stacks.",
	Long: `The 'atmos terraform generate' command is used to generate Terraform configuration files
for specific components and stacks within your Atmos setup.

This command supports the following subcommands:
- 'backend' to generate a backend configuration file for an Atmos component in a stack.
- 'backends' to generate backend configuration files for all Atmos components in all stacks.
- 'varfile' to generate a variable file (varfile) for an Atmos component in a stack.
- 'varfiles' to generate varfiles for all Atmos components in all stacks.`,
	Args: cobra.NoArgs,
}

func init() {
	terraformCmd.AddCommand(terraformGenerateCmd)
}
