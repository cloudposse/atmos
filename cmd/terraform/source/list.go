package source

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List components with source in a stack",
	Long: `List all terraform components that have source configured in a stack.

This command shows which components can be vendored using the source provisioner.`,
	Example: `  # List components with source
  atmos terraform source list --stack dev`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return executeListCommand(cmd)
	},
}

func init() {
	listCmd.DisableFlagParsing = false

	listParser = flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	listParser.RegisterFlags(listCmd)

	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListCommand(cmd *cobra.Command) error {
	defer perf.Track(nil, "source.list.RunE")()

	// Parse flags.
	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	stack := v.GetString("stack")
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			Err()
	}

	// TODO: Implement list functionality.
	// This should:
	// 1. Load all components in the stack
	// 2. Filter to those with source configured
	// 3. Display in a table format
	return errUtils.Build(errUtils.ErrNotImplemented).
		WithExplanation("List sources functionality is not yet implemented").
		WithHint("This feature is planned for a future release").
		Err()
}
