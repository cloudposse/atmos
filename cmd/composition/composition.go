package composition

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	pkgcomposition "github.com/cloudposse/atmos/pkg/composition"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

var compositionParser *flags.StandardParser

// compositionCmd is the base command for composition operations.
var compositionCmd = &cobra.Command{
	Use:   "composition",
	Short: "Inspect compositions that group component instances into systems",
	Long:  "Operate on compositions — named groupings of component instances (services) that form a system.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
}

// The validate subcommand reports a composition's fulfilled and not-provided services.
var validateCmd = &cobra.Command{
	Use:   "validate <composition>",
	Short: "Report a composition's fulfilled and not-provided services for a stack",
	Long:  "Show which of a composition's declared services are fulfilled by components in a stack and which are not provided there.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		info := buildConfigAndStacksInfo(cmd)
		return pkgcomposition.ExecuteValidate(cmd.Context(), &info, args[0])
	},
}

func init() {
	compositionParser = flags.NewStandardParser(flags.WithFlagRegistry(flags.CommonFlags()))
	compositionParser.RegisterPersistentFlags(compositionCmd)
	if err := compositionParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	compositionCmd.AddCommand(validateCmd)
	internal.Register(&CompositionCommandProvider{})
}

// CompositionCommandProvider implements the CommandProvider interface.
type CompositionCommandProvider struct{}

// GetCommand returns the composition command.
func (c *CompositionCommandProvider) GetCommand() *cobra.Command { return compositionCmd }

// GetName returns the command name.
func (c *CompositionCommandProvider) GetName() string { return "composition" }

// GetGroup returns the command group for help organization.
func (c *CompositionCommandProvider) GetGroup() string { return "Core Stack Commands" }

// GetAliases returns command aliases.
func (c *CompositionCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// GetFlagsBuilder returns the flags builder for this command.
func (c *CompositionCommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (c *CompositionCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (c *CompositionCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (c *CompositionCommandProvider) IsExperimental() bool { return false }

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo from global + stack flags.
func buildConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	info := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		Identity:                cfg.NormalizeIdentityValue(globalFlags.Identity.Value()),
		ProfilesFromArg:         globalFlags.Profile,
	}
	if stackFlag := cmd.Flag("stack"); stackFlag != nil && stackFlag.Value.String() != "" {
		info.Stack = stackFlag.Value.String()
	}
	return info
}
