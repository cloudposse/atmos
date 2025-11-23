package generate

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ui"
)

// varfilesCmd generates varfiles for all terraform components in all stacks.
var varfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Generate varfiles for all Terraform components in all stacks",
	Long:               "This command generates varfiles for all Atmos Terraform components across all stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		return err
	},
}

func init() {
	varfilesCmd.DisableFlagParsing = false

	varfilesCmd.PersistentFlags().String("file-template", "",
		"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders.",
	)

	varfilesCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names",
	)

	varfilesCmd.PersistentFlags().String("components", "",
		"Only generate the `.tfvar` files for the specified `atmos` components (use comma-separated values).",
	)

	varfilesCmd.PersistentFlags().String("format", "hcl", "Specify the output format. Supported formats: `hcl`, `json`, `backend-config` (`hcl` is default).")

	if err := varfilesCmd.MarkPersistentFlagRequired("file-template"); err != nil {
		ui.Error(err.Error())
	}

	GenerateCmd.AddCommand(varfilesCmd)
}
