package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateVarfilesCmd generates varfiles for all terraform components in all stacks
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Execute 'terraform generate varfiles' command",
	Long:               `This command generates varfiles for all terraform components in all stacks`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateVarfilesCmd(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateVarfilesCmd.DisableFlagParsing = false
	terraformGenerateVarfilesCmd.PersistentFlags().String("path-template", "", "atmos terraform generate varfiles --path-template <path_template>")
	terraformGenerateVarfilesCmd.PersistentFlags().String("stacks", "", "Filter by specific stacks: atmos terraform generate varfiles --path-template <path_template> --stacks <stack1>,<stack2>")
	terraformGenerateVarfilesCmd.PersistentFlags().String("components", "", "Filter by specific components: atmos terraform generate varfiles --path-template <path_template> --components=<component1>,<component2>")

	err := terraformGenerateVarfilesCmd.MarkPersistentFlagRequired("path-template")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
