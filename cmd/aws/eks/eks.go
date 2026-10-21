package eks

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// EksCmd executes 'aws eks' CLI commands.
var EksCmd = &cobra.Command{
	Use:   "eks",
	Short: "Run AWS EKS CLI commands for cluster management",
	Long: `Manage Amazon EKS clusters using AWS CLI, including configuring kubeconfig and performing cluster-related operations.

You can use this command to interact with AWS EKS, including operations like configuring kubeconfig, managing clusters, and more.

For a list of available AWS EKS commands, refer to the Atmos documentation:
https://atmos.tools/cli/commands/aws/eks-update-kubeconfig`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	internal.RegisterCommandMetadata(
		EksCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/aws/eks/eks"),
	)
}
