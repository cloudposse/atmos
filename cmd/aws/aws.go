package aws

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/aws/eks"
	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// doubleDashHint is displayed in help output.
const doubleDashHint = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."

// awsCmd executes 'aws' CLI commands.
var awsCmd = &cobra.Command{
	Use:                "aws",
	Short:              "Run AWS-specific commands for interacting with cloud resources",
	Long:               `This command allows interaction with AWS resources through various CLI commands.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	awsCmd.PersistentFlags().Bool("", false, doubleDashHint)

	// Add EKS subcommand from the eks subpackage.
	awsCmd.AddCommand(eks.EksCmd)

	// Register this command with the registry.
	internal.Register(&AWSCommandProvider{})
}

// AWSCommandProvider implements the CommandProvider interface.
type AWSCommandProvider struct{}

// GetCommand returns the aws command.
func (a *AWSCommandProvider) GetCommand() *cobra.Command {
	return awsCmd
}

// GetName returns the command name.
func (a *AWSCommandProvider) GetName() string {
	return "aws"
}

// GetGroup returns the command group for help organization.
func (a *AWSCommandProvider) GetGroup() string {
	return "Cloud Integration"
}

// GetAliases returns command aliases.
func (a *AWSCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for aws command.
}

// GetFlagsBuilder returns the flags builder for this command.
func (a *AWSCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (a *AWSCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil // AWS command has subcommands, not positional args.
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (a *AWSCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
