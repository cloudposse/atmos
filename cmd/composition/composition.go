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

const (
	stackFlagName = "stack"
	tailFlagName  = "tail"
)

var lifecycleRequiredFlags = func() *flags.FlagRegistry {
	registry := flags.NewFlagRegistry()
	registry.RegisterStringFlag(stackFlagName, "s", "", "Atmos stack", true)
	return registry
}()

var lifecycleVerbs = []string{"up", "start", "restart", "stop", "rm", "down", "ps"}

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
	Use:   "validate [composition]",
	Short: "Report a composition's fulfilled and not-provided services for a stack",
	Long:  "Show which declared services are fulfilled by components in a stack and which are not provided there.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := compositionParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
			return err
		}
		info := buildConfigAndStacksInfo(cmd)
		return pkgcomposition.ExecuteValidate(cmd.Context(), &info, optionalCompositionArg(args))
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List declared compositions and stack fulfillment",
	Long:  "List declared compositions. When --stack is set, include fulfilled and not-provided services for that stack.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runList(cmd)
	},
}

func newLifecycleCmd(verb string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   verb + " [composition]",
		Short: verb + " composition members in a stack",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLifecycle(cmd, args, verb, nil)
		},
	}
	return cmd
}

var logsCmd = &cobra.Command{
	Use:   "logs [composition]",
	Short: "Show logs from composition members in a stack",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLifecycle(cmd, args, "logs", compositionVerbFlags(cmd))
	},
}

func init() {
	compositionParser = flags.NewStandardParser(
		flags.WithFlagRegistry(flags.CommonFlags()),
		flags.WithCompletionPrompt(stackFlagName, "Choose a stack", compositionStackFlagCompletion),
	)
	compositionParser.Registry().SetCompletionFunc(stackFlagName, compositionStackFlagCompletion)
	compositionParser.RegisterPersistentFlags(compositionCmd)
	if err := compositionParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().String(tailFlagName, "all", "Number of lines to show from the end of the logs, or \"all\"")
	logsCmd.Flags().Lookup(tailFlagName).NoOptDefVal = "all"

	commands := []*cobra.Command{listCmd, validateCmd, logsCmd}
	for _, verb := range lifecycleVerbs {
		commands = append(commands, newLifecycleCmd(verb))
	}
	compositionCmd.AddCommand(commands...)
	internal.Register(&CompositionCommandProvider{})
}

// runLifecycle binds shared flags, asks for a missing stack in an interactive
// terminal, validates the lifecycle target, then dispatches to the composition
// executor.
func runLifecycle(cmd *cobra.Command, args []string, verb string, verbFlags map[string]any) error {
	if err := compositionParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return err
	}
	parsed, err := compositionParser.Parse(cmd.Context(), args)
	if err != nil {
		return err
	}
	if err := lifecycleRequiredFlags.Validate(map[string]interface{}{stackFlagName: parsed.Stack}); err != nil {
		return err
	}
	info := buildConfigAndStacksInfo(cmd)
	info.Stack = parsed.Stack
	return pkgcomposition.ExecuteLifecycle(cmd.Context(), &info, verb, optionalCompositionArg(args), verbFlags)
}

func optionalCompositionArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func compositionVerbFlags(cmd *cobra.Command) map[string]any {
	flagValues := map[string]any{}
	if f := cmd.Flag("follow"); f != nil {
		flagValues["follow"] = f.Value.String() == "true"
	}
	if f := cmd.Flag(tailFlagName); f != nil {
		flagValues[tailFlagName] = f.Value.String()
	}
	return flagValues
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
	// Prefer the Cobra flag so an interactive stack selection is used. Fall back
	// to Viper to retain the flag > environment > configuration precedence chain.
	if stackFlag := cmd.Flag(stackFlagName); stackFlag != nil && stackFlag.Value.String() != "" {
		info.Stack = stackFlag.Value.String()
	} else if stack := viper.GetViper().GetString(stackFlagName); stack != "" {
		info.Stack = stack
	}
	if dryRunFlag := cmd.Flag("dry-run"); dryRunFlag != nil && dryRunFlag.Value.String() == "true" {
		info.DryRun = true
	}
	return info
}
