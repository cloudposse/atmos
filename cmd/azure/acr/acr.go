package acr

import "github.com/spf13/cobra"

// AcrCmd executes 'azure acr' CLI commands.
var AcrCmd = &cobra.Command{
	Use:   "acr",
	Short: "Manage Azure Container Registry authentication and registry access",
	Long: `Manage authentication for Azure Container Registry (ACR).

Login to ACR registries using named integrations, identity-linked integrations,
or explicit registry URLs. Credentials are written to Docker config for seamless
container workflows.

For more information, refer to the Atmos documentation:
https://atmos.tools/cli/commands/azure/acr-login`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}
