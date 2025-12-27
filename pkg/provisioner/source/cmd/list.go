package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ListCommand creates a list command for the given component type.
func ListCommand(cfg *Config) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %s components with source in a stack", cfg.TypeLabel),
		Long: fmt.Sprintf(`List all %s components that have source configured in a stack.

This command shows which components can be vendored using the source provisioner.`, cfg.TypeLabel),
		Example: fmt.Sprintf(`  # List components with source
  atmos %s source list --stack dev`, cfg.ComponentType),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeList(cmd, cfg, parser)
		},
	}

	cmd.DisableFlagParsing = false
	parser.RegisterFlags(cmd)

	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	return cmd
}

func executeList(cmd *cobra.Command, cfg *Config, parser *flags.StandardParser) error {
	defer perf.Track(nil, fmt.Sprintf("source.%s.list.RunE", cfg.ComponentType))()

	// Parse flags.
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
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
	// 1. Load all components in the stack.
	// 2. Filter to those with source configured.
	// 3. Display in a table format.
	return errUtils.Build(errUtils.ErrNotImplemented).
		WithExplanation("List sources functionality is not yet implemented").
		WithHint("This feature is planned for a future release").
		Err()
}
