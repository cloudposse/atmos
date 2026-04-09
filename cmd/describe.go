package cmd

import (
	"github.com/spf13/cobra"
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
	// NOTE: NoOptDefVal is NOT used here to avoid Cobra parsing issues with commands
	// that have positional arguments. When NoOptDefVal is set and a space-separated value
	// is used (--identity value), Cobra misinterprets the value as a subcommand/positional arg.
	// Users should use --identity=select or similar for interactive selection.
	describeCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate with before describing")

	// Register shell completion for identity flag.
	AddIdentityCompletion(describeCmd)

	RootCmd.AddCommand(describeCmd)
}
