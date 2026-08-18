package toolchain

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var (
	listParser           *flags.StandardParser
	supportedListFormats = []string{"table", "plain", "json"}
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured tools and their installation status",
	Long:  `List all tools configured in .tool-versions file, showing their installation status, install date, and file size.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		format := v.GetString("format")
		installedOnly := v.GetBool("installed-only")
		pendingOnly := v.GetBool("pending-only")

		if !slices.Contains(supportedListFormats, format) {
			return fmt.Errorf("%w: %q (supported: %v)", errUtils.ErrInvalidFlagValue, format, supportedListFormats)
		}
		if installedOnly && pendingOnly {
			return errUtils.Build(errUtils.ErrMutuallyExclusiveFlags).
				WithExplanation("--installed-only and --pending-only cannot be used together").
				WithHint("Choose one filter, or omit both to list every tool").
				Err()
		}

		return toolchain.RunListWithOptions(format, installedOnly, pendingOnly)
	},
}

func init() {
	// Create parser with list-specific flags.
	listParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "", "table", fmt.Sprintf("Output format: %v", supportedListFormats)),
		flags.WithBoolFlag("installed-only", "", false, "Show only installed tools"),
		flags.WithBoolFlag("pending-only", "", false, "Show only pending tools that are not yet installed"),
		flags.WithEnvVars("format", "ATMOS_TOOLCHAIN_FORMAT"),
		flags.WithEnvVars("installed-only", "ATMOS_TOOLCHAIN_INSTALLED_ONLY"),
		flags.WithEnvVars("pending-only", "ATMOS_TOOLCHAIN_PENDING_ONLY"),
	)

	// Register flags.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register shell completion for the format flag.
	if err := listCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return supportedListFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(err)
	}
}

// ListCommandProvider implements the CommandProvider interface.
type ListCommandProvider struct{}

func (l *ListCommandProvider) GetCommand() *cobra.Command {
	return listCmd
}

func (l *ListCommandProvider) GetName() string {
	return "list"
}

func (l *ListCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (l *ListCommandProvider) GetFlagsBuilder() flags.Builder {
	return listParser
}

func (l *ListCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (l *ListCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
