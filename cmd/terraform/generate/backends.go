package generate

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// backendsCmd generates backend configs for all terraform components.
var backendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Generate backend configurations for all Terraform components",
	Long:               "This command generates the backend configuration files for all Terraform components in the Atmos environment.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		return err
	},
}

func init() {
	backendsCmd.DisableFlagParsing = false

	backendsCmd.PersistentFlags().String("file-template", "",
		"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders.",
	)

	backendsCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names",
	)

	backendsCmd.PersistentFlags().String("components", "",
		"Only generate the backend files for the specified `atmos` components (comma-separated values).",
	)

	backendsCmd.PersistentFlags().String("format", "hcl", "Specify the output format. Supported formats: `hcl`, `json`, `backend-config` (`hcl` is default).")

	GenerateCmd.AddCommand(backendsCmd)
}
