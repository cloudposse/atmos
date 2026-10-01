package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var providersParser *flags.StandardParser

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
		opts, err := parseTerraformRunOptions(cmd, providersParser)
		if err != nil {
			return err
		}
		return terraformRunWithOptions(terraformCmd, cmd, args, opts)
	},
}

// providersSubcmds defines the terraform providers sub-subcommands.
// Each entry is registered as a Cobra child command of providersCmd,
// enabling proper command tree routing instead of hardcoded argument parsing.
// The compatFunc provides per-subcommand compat flags for the command registry.
var providersSubcmds = []struct {
	name       string
	short      string
	compatFunc func() map[string]compat.CompatibilityFlag
}{
	{"lock", "Write out dependency locks for the configured providers", ProvidersLockCompatFlags},
	{"mirror", "Save local copies of all required provider plugins", ProvidersMirrorCompatFlags},
	{"schema", "Show schemas for the providers used in the configuration", ProvidersSchemaCompatFlags},
}

func init() {
	// Providers commands can run against a fleet just like plan and init. These
	// flags must live on the providers parent so its nested commands inherit and
	// Cobra parses them before passthrough execution.
	providersParser = flags.NewStandardParser(
		flags.WithBoolFlag("affected", "", false, "Run for affected components in dependency order"),
		flags.WithBoolFlag("all", "", false, "Run for all components in all stacks"),
		flags.WithIntFlag("max-concurrency", "", 1, "Maximum number of components to execute concurrently"),
		flags.WithStringFlag("failure-mode", "", terraformFailureModeFailFast, "Fleet execution failure handling mode. Supported values: fail-fast, keep-going"),
		flags.WithStringFlag("log-order", "", "stream", "Order concurrent execution logs. Supported values: stream, grouped"),
	)
	providersParser.RegisterPersistentFlags(providersCmd)
	if err := providersParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register sub-subcommands for providers (e.g., "providers lock", "providers mirror").
	for _, sub := range providersSubcmds {
		cmd := newTerraformPassthroughSubcommandWithParsers(providersCmd, sub.name, sub.short, providersParser)
		// Cobra's unknown-flag passthrough on compound Terraform commands does
		// not reliably merge the parent's persistent flags into the leaf before
		// parsing. Register the fleet flags directly on each leaf so `lock --all`
		// and `lock --affected` are parsed rather than forwarded as Terraform
		// arguments.
		providersParser.RegisterFlags(cmd)
		providersCmd.AddCommand(cmd)
		internal.RegisterCommandCompatFlags("terraform", "providers-"+sub.name, sub.compatFunc())
	}

	// Register completions for providersCmd.
	RegisterTerraformCompletions(providersCmd)

	// Register compat flags for this subcommand.
	internal.RegisterCommandCompatFlags("terraform", "providers", ProvidersCompatFlags())

	// Attach to parent terraform command.
	terraformCmd.AddCommand(providersCmd)
}
