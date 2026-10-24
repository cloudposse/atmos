package aks

import "github.com/spf13/cobra"

// AksCmd executes 'azure aks' CLI commands.
var AksCmd = &cobra.Command{
	Use:   "aks",
	Short: "Run Azure AKS CLI commands for cluster management",
	Long: `Manage Azure Kubernetes Service (AKS) clusters, including configuring kubeconfig and
generating bearer tokens for kubectl.

For a list of available Azure AKS commands, refer to the Atmos documentation:
https://atmos.tools/cli/commands/azure/aks/update-kubeconfig`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}
