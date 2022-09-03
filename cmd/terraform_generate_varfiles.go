package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// terraformGenerateVarfileCmd generates varfile for a terraform component
var terraformGenerateVarfilesCmd = &cobra.Command{
	Use:                "varfiles",
	Short:              "Execute 'terraform generate varfiles' command",
	Long:               `This command generates varfiles for all terraform components in all stacks`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteTerraformGenerateVarfiles(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	terraformGenerateVarfilesCmd.DisableFlagParsing = false
	terraformGenerateVarfilesCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform generate varfiles <component> -s <stack>")
	terraformGenerateVarfilesCmd.PersistentFlags().String("file", "", "atmos terraform generate varfiles --file <file_template>")
	terraformGenerateVarfilesCmd.PersistentFlags().String("format", "yaml", "Specify output format: atmos terraform generate varfiles --format=yaml/json ('json' is default)")
	terraformGenerateVarfilesCmd.PersistentFlags().String("components", "", "Filter by specific components: atmos terraform generate varfiles --components=<component1>,<component2>")

	err := terraformGenerateVarfilesCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	terraformGenerateCmd.AddCommand(terraformGenerateVarfilesCmd)
}
