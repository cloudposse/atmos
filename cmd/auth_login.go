package cmd

import (
	"github.com/cloudposse/atmos/internal/auth"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// awsEksCmdUpdateKubeconfigCmd executes 'aws eks update-kubeconfig' command
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "",
	Long:  ``,

	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		err := auth.ExecuteAuthLoginCommand(cmd, args)
		if err != nil {
			u.PrintfMarkdown("", err, "")
		}
	},
}

// https://docs.aws.amazon.com/cli/latest/reference/eks/update-kubeconfig.html
func init() {
	AddStackCompletion(authLoginCmd)
	authCmd.AddCommand(authLoginCmd)
}
