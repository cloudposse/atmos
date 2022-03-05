package exec

import (
	"github.com/spf13/cobra"
)

// ExecuteTerraformGenerateVarfile executes `terraform generate varfile` command
func ExecuteTerraformGenerateVarfile(cmd *cobra.Command, args []string) error {
	return generateVarfile(cmd, args, "terraform")
}
