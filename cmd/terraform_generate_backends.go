package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// terraformGenerateBackendsCmd generates backend configs for all terraform components
var terraformGenerateBackendsCmd = &cobra.Command{
	Use:                "backends",
	Short:              "Generate backend configurations for all Terraform components",
	Long:               "This command generates the backend configuration files for all Terraform components in the Atmos environment.",
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateBackendsCmd(cmd, args)
		return err
	},
}

func init() {
	terraformGenerateBackendsCmd.DisableFlagParsing = false

	terraformGenerateBackendsCmd.PersistentFlags().String("file-template", "",
		"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders.",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("components", "",
		"Only generate the backend files for the specified `atmos` components (comma-separated values).",
	)

	terraformGenerateBackendsCmd.PersistentFlags().String("format", "hcl", "Specify the output format. Supported formats: `hcl`, `json`, `backend-config` (`hcl` is default).")

	terraformGenerateCmd.AddCommand(terraformGenerateBackendsCmd)
}
