package terraform

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// providersCmd represents the terraform providers command.
var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Show the providers required for this configuration",
	Long: `Prints a tree of the providers used in the configuration.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/providers
  https://opentofu.org/docs/cli/commands/providers`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

// providersSubcmds defines the terraform providers sub-subcommands.
// Each entry is registered as a Cobra child command of providersCmd,
// enabling proper command tree routing instead of hardcoded argument parsing.
var providersSubcmds = []struct {
	name  string
	short string
}{
	{"lock", "Write out dependency locks for the configured providers"},
	{"mirror", "Save local copies of all required provider plugins"},
	{"schema", "Show schemas for the providers used in the configuration"},
}

func init() {
	// Register sub-subcommands for providers (e.g., "providers lock", "providers mirror").
	for _, sub := range providersSubcmds {
		providersCmd.AddCommand(newTerraformPassthroughSubcommand(providersCmd, sub.name, sub.short))
	}

	// Register completions for providersCmd.
	RegisterTerraformCompletions(providersCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "providers", ProvidersCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(providersCmd)
}
