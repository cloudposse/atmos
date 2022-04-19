package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// awsEksCmdUpdateKubeconfigCmd executes 'aws eks update-kubeconfig' command
// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
var awsEksCmdUpdateKubeconfigCmd = &cobra.Command{
	Use:                "update-kubeconfig",
	Short:              "Execute 'aws eks update-kubeconfig' command",
	Long:               `This command executes 'aws eks update-kubeconfig' command. Docs: https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteAwsEksUpdateKubeconfigCommand(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	awsEksCmdUpdateKubeconfigCmd.DisableFlagParsing = false
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().StringP("stack", "s", "", "atmos aws eks update-kubeconfig -s <stack>")
	awsEksCmdUpdateKubeconfigCmd.PersistentFlags().String("stack", "", "atmos aws eks update-kubeconfig -s <stack>")

	awsEksCmd.AddCommand(awsEksCmdUpdateKubeconfigCmd)
}
