package generate

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/terraform/generate/required_providers"
)

// GenerateCmd is the parent command for all terraform generate subcommands.
// It is exported so the terraform package can add it as a subcommand.
var GenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Terraform configuration files for Atmos components and stacks",
	Long: `The 'atmos terraform generate' command is used to generate Terraform configuration files
for specific components and stacks within your Atmos setup.

This command supports the following subcommands:
- 'backend' to generate a backend configuration file for an Atmos component in a stack.
- 'backends' to generate backend configuration files for all Atmos components in all stacks.
- 'varfile' to generate a variable file (varfile) for an Atmos component in a stack.
- 'varfiles' to generate varfiles for all Atmos components in all stacks.
- 'planfile' to generate a planfile for an Atmos component in a stack.
- 'required-providers' to generate a terraform_override.tf.json file with required_version and required_providers.
- 'files' to generate files from the generate section for an Atmos component.`,
	Args:               cobra.NoArgs,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	// Add required-providers command (DEV-3124).
	GenerateCmd.AddCommand(required_providers.NewRequiredProvidersCommand())
}
