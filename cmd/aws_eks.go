package cmd

import (
	"github.com/spf13/cobra"
)

// awsCmd executes 'aws eks' CLI commands
var awsEksCmd = &cobra.Command{
	Use:   "eks",
	Short: "Run AWS EKS CLI commands for cluster management",
	Long: `This command allows you to execute various 'aws eks' CLI commands for managing Amazon EKS clusters.

	You can use this command to interact with AWS EKS, including operations like configuring kubeconfig, managing clusters, and more.
	
	For a list of available AWS EKS commands, refer to the AWS CLI documentation:
	https://docs.aws.amazon.com/cli/latest/reference/eks/`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
}

func init() {
	awsCmd.AddCommand(awsEksCmd)
}
