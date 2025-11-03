package cmd

import (
	"github.com/spf13/cobra"
)

// describeCmd describes configuration for stacks and components.
var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show details about Atmos configurations and components",
	Long:  `Display configuration details for Atmos CLI, stacks, and components.`,
	Args:  cobra.NoArgs,
}

func init() {
	describeCmd.PersistentFlags().StringP("query", "q", "", "Query the results of an `atmos describe` command using `yq` expressions")

	// Add --identity flag to all describe commands to enable authentication
	// when processing YAML template functions (!terraform.state, !terraform.output).
	// By default, all describe commands execute YAML functions and Go templates unless
	// disabled with --process-functions=false or --process-templates=false flags.
	describeCmd.PersistentFlags().StringP("identity", "i", "", "Specify the identity to authenticate to before describing. Use without value to interactively select.")

	// Set NoOptDefVal to enable optional flag value for interactive identity selection.
	// When --identity is used without a value, it will receive IdentityFlagSelectValue.
	if identityFlag := describeCmd.PersistentFlags().Lookup("identity"); identityFlag != nil {
		identityFlag.NoOptDefVal = IdentityFlagSelectValue
	}

	// Register shell completion for identity flag.
	AddIdentityCompletion(describeCmd)

	RootCmd.AddCommand(describeCmd)
}
