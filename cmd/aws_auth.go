package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// awsAuthCmd executes 'aws sso login' command
var awsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authenticate to AWS using SSO",
	Long: `This command performs AWS SSO login based on your Atmos configuration.
It resolves the AWS profile from the provided component and stack context and runs \"aws sso login\".`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteAwsAuthCommand(cmd, args)
		if err != nil {
			u.PrintfMarkdown("", err, "")
		}
	},
}

func init() {
	AddStackCompletion(awsAuthCmd)
	awsAuthCmd.PersistentFlags().String("profile", "", "Specify the AWS CLI profile to use for authentication")
	awsCmd.AddCommand(awsAuthCmd)
}
