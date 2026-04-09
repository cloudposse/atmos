package ecr

import "github.com/spf13/cobra"

// EcrCmd executes 'aws ecr' CLI commands.
var EcrCmd = &cobra.Command{
	Use:   "ecr",
	Short: "Manage AWS ECR authentication and registry access",
	Long: `Manage authentication for Amazon Elastic Container Registry (ECR).

Login to ECR registries using named integrations, identity-linked integrations,
or explicit registry URLs. Credentials are written to Docker config for seamless
container workflows.

For more information, refer to the Atmos documentation:
https://atmos.tools/cli/commands/aws/ecr-login`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}
