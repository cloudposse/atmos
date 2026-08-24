// Package required_providers provides CLI commands for generating Terraform required_providers blocks.
package required_providers

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// NewRequiredProvidersCommand creates the required-providers command.
// This command generates a terraform_override.tf.json file with required_version
// and required_providers blocks from stack configuration (DEV-3124).
func NewRequiredProvidersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "required-providers",
		Short: "Generate a required_providers block for a Terraform component",
		Long: `This command generates a 'terraform_override.tf.json' file for a specified Atmos Terraform component.

The file contains the required_version and required_providers blocks from the component's stack configuration,
allowing you to pin Terraform and provider versions per component.

Example stack configuration:

  terraform:
    required_version: ">= 1.10.1"
    required_providers:
      aws:
        source: "hashicorp/aws"
        version: "~> 5.0"
      kubernetes:
        source: "hashicorp/kubernetes"
        version: "~> 2.0"`,
		Args:               cobra.ExactArgs(1),
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		RunE:               e.ExecuteTerraformGenerateRequiredProvidersCmd,
	}

	// Add flags.
	cmd.Flags().StringP("stack", "s", "", "The stack to use for component generation")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringP("file", "f", "", "Specify the path to the file to generate (default: terraform_override.tf.json in component directory)")

	return cmd
}
