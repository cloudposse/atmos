package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// helmfileGenerateVarfileCmd generates varfile for a helmfile component
var helmfileGenerateVarfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Execute 'helmfile generate varfile' command",
	Long:               `This command generates a varfile for an atmos helmfile component: atmos helmfile generate varfile <component> -s <stack> -f <file>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteHelmfileGenerateVarfile(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	helmfileGenerateVarfileCmd.DisableFlagParsing = false
	helmfileGenerateVarfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile generate varfile <component> -s <stack>")
	helmfileGenerateVarfileCmd.PersistentFlags().StringP("file", "f", "", "atmos helmfile generate varfile <component> -s <stack> -f <file>")

	err := helmfileGenerateVarfileCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	helmfileGenerateCmd.AddCommand(helmfileGenerateVarfileCmd)
}
