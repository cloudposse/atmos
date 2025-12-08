package varfile

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// NewVarfileCommand creates the varfile command using modern patterns.
// This command benefits from proper I/O context initialization in root.go PersistentPreRun.
func NewVarfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "varfile",
		Short:              "Generate a varfile for a Terraform component",
		Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
		Args:               cobra.ExactArgs(1),
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		RunE:               e.ExecuteTerraformGenerateVarfileCmd,
	}

	// Add flags.
	cmd.Flags().StringP("stack", "s", "", "The stack to use for component generation")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack")

	return cmd
}
