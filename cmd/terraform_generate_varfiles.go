package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformGenerateVarfilesCmd generates varfiles for all terraform components in all stacks
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Generate varfiles for all Terraform components in all stacks",
	Long:               "This command generates varfiles for all Atmos Terraform components across all stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	terraformGenerateVarfilesCmd.DisableFlagParsing = false

	terraformGenerateVarfilesCmd.PersistentFlags().String("file-template", "",
		"Template for generating backend configuration files, supporting absolute/relative paths and context tokens (e.g., {tenant}, {environment}, {component}). Subdirectories are created automatically. If not specified, files are written to corresponding Terraform component folders.",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("stacks", "",
		"Only process the specified stacks (comma-separated values), supporting top-level stack manifest paths or derived Atmos stack names",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("components", "",
		"Only generate the `.tfvar` files for the specified `atmos` components (use comma-separated values).",
	)

	terraformGenerateVarfilesCmd.PersistentFlags().String("format", "hcl", "Specify the output format. Supported formats: `hcl`, `json`, `backend-config` (`hcl` is default).")

	err := terraformGenerateVarfilesCmd.MarkPersistentFlagRequired("file-template")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
