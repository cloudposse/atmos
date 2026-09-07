package cmd

import (
	"github.com/spf13/cobra"

	pkgFlags "github.com/cloudposse/atmos/pkg/flags"
)

// describeCmd describes configuration for stacks and components.
var describeCmd = &cobra.Command{
	Use:                "describe",
	Short:              "Show details about Atmos configurations and components",
	Long:               `Display configuration details for Atmos CLI, stacks, and components.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
}

func init() {
	describeCmd.PersistentFlags().StringP("query", "q", "", "Query the results of an `atmos describe` command using `yq` expressions")

	// Add --identity flag to all describe commands to enable authentication
	// when processing YAML template functions (!terraform.state, !terraform.output).
	// By default, all describe commands execute YAML functions and Go templates unless
	// disabled with --process-functions=false or --process-templates=false flags.
	//
	// Uses the shared flags.WithIdentityFlag() builder (rather than a hand-rolled
	// PersistentFlags().StringP()) so bare --identity triggers the interactive selector like
	// every other Atmos command. Space-separated values on subcommands with positional args
	// (e.g. `describe component vpc --identity foo`) are normalized to `--identity=foo` by the
	// generic NoOptDefVal preprocessor in cmd/root.go before Cobra parses, and identity
	// retrieval already goes through GetIdentityFromFlags for extra safety.
	pkgFlags.NewStandardParser(pkgFlags.WithIdentityFlag()).RegisterPersistentFlags(describeCmd)

	// Register shell completion for identity flag.
	AddIdentityCompletion(describeCmd)

	RootCmd.AddCommand(describeCmd)
}
