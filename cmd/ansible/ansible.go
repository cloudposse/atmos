package ansible

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ansibleParser handles flag parsing for shared ansible flags.
// These persistent flags are inherited by all ansible subcommands.
var ansibleParser *flags.StandardParser

// ansibleCmd represents the base command for all ansible sub-commands.
var ansibleCmd = &cobra.Command{
	Use:     "ansible",
	Aliases: []string{"an"},
	Short:   "Manage ansible-based automation for infrastructure configuration",
	Long:    `Run Ansible commands for automating infrastructure configuration, application deployment, and orchestration.`,
	// FParseErrWhitelist allows unknown flags to pass through to Ansible.
	// Unlike DisableFlagParsing, this still allows Cobra to parse known Atmos flags.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	// RunE handles the case when ansible is called without a subcommand.
	RunE: ansibleGlobalFlagsHandler,
}

func init() {
	// Create parser with shared ansible flags using functional options.
	// These flags are inherited by all ansible subcommands.
	ansibleParser = flags.NewStandardParser(
		WithAnsibleFlags(),
	)

	// Set stack completion function on the flag registry to avoid import cycle.
	// This must be done before RegisterPersistentFlags() so the completion
	// function is registered when the flag is registered.
	ansibleParser.Registry().SetCompletionFunc("stack", stackFlagCompletion)

	// Register as persistent flags (inherited by subcommands).
	ansibleParser.RegisterPersistentFlags(ansibleCmd)

	// Bind flags to Viper for environment variable support.
	if err := ansibleParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Normalize only the long identity flag for Ansible. The -i shorthand belongs
	// to Ansible inventory, so Atmos identity must remain long-form here.
	identityRegistry := flags.NewFlagRegistry()
	identityRegistry.Register(&flags.StringFlag{
		Name:        cfg.IdentityFlagName,
		Default:     "",
		Description: "Specify the identity to authenticate before running Ansible commands.",
		NoOptDefVal: cfg.IdentityFlagSelectValue,
	})
	internal.RegisterCommandFlagRegistry("ansible", identityRegistry)

	// Add subcommands.
	ansibleCmd.AddCommand(playbookCmd)
	ansibleCmd.AddCommand(versionCmd)

	// Register completion functions for component argument.
	RegisterAnsibleCompletions(ansibleCmd)

	// Register this command with the registry.
	internal.Register(&AnsibleCommandProvider{})
}

// AnsibleCommandProvider implements the CommandProvider interface.
type AnsibleCommandProvider struct{}

// GetCommand returns the ansible command.
func (a *AnsibleCommandProvider) GetCommand() *cobra.Command {
	return ansibleCmd
}

// GetName returns the command name.
func (a *AnsibleCommandProvider) GetName() string {
	return "ansible"
}

// GetGroup returns the command group for help organization.
func (a *AnsibleCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// GetAliases returns command aliases.
func (a *AnsibleCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for ansible command.
}

// GetFlagsBuilder returns the flags builder for this command.
func (a *AnsibleCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil // Flags are handled by ansibleParser.
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (a *AnsibleCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil // Ansible command has subcommands, not positional args.
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (a *AnsibleCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil // No compatibility flags for ansible.
}

// IsExperimental returns whether this command is experimental.
func (a *AnsibleCommandProvider) IsExperimental() bool {
	return false
}

// ansibleGlobalFlagsHandler handles the ansible command when called without a subcommand.
func ansibleGlobalFlagsHandler(cmd *cobra.Command, args []string) error {
	// No global flag found and no subcommand provided - show usage.
	return cmd.Usage()
}

func resolveAnsibleIdentity(cmd *cobra.Command, parsedIdentity string) string {
	if parsedIdentity != "" {
		return parsedIdentity
	}

	if value, ok := getChangedIdentityFlagValue(cmd); ok {
		return cfg.NormalizeIdentityValue(value)
	}

	if value, ok := getLongIdentityFromArgs(os.Args[1:]); ok {
		return cfg.NormalizeIdentityValue(value)
	}

	if envIdentity := os.Getenv("ATMOS_IDENTITY"); envIdentity != "" {
		return cfg.NormalizeIdentityValue(envIdentity)
	}

	return ""
}

func getChangedIdentityFlagValue(cmd *cobra.Command) (string, bool) {
	for current := cmd; current != nil; current = current.Parent() {
		for _, flagSet := range []*pflag.FlagSet{
			current.Flags(),
			current.InheritedFlags(),
			current.PersistentFlags(),
		} {
			if flagSet == nil {
				continue
			}

			flag := flagSet.Lookup(cfg.IdentityFlagName)
			if flag == nil || !flag.Changed {
				continue
			}

			return flag.Value.String(), true
		}
	}

	return "", false
}

func getLongIdentityFromArgs(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return "", false
		}

		if arg == cfg.IdentityFlag {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				return args[i+1], true
			}
			return cfg.IdentityFlagSelectValue, true
		}

		if strings.HasPrefix(arg, cfg.IdentityFlag+"=") {
			value := strings.TrimPrefix(arg, cfg.IdentityFlag+"=")
			if value == "" {
				return cfg.IdentityFlagSelectValue, true
			}
			return value, true
		}
	}

	return "", false
}

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo with global flags populated.
// This ensures config selection flags (--base-path, --config, --config-path, --profile)
// are properly honored when initializing CLI config.
func buildConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)

	info := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		Identity:                resolveAnsibleIdentity(cmd, globalFlags.Identity.Value()),
		ProfilesFromArg:         globalFlags.Profile,
	}

	// Get stack from flag if provided.
	if stackFlag := cmd.Flag("stack"); stackFlag != nil && stackFlag.Value.String() != "" {
		info.Stack = stackFlag.Value.String()
	}

	// Get dry-run from flag if provided.
	if dryRunFlag := cmd.Flag("dry-run"); dryRunFlag != nil && dryRunFlag.Value.String() == "true" {
		info.DryRun = true
	}

	return info
}

// ansibleCLIFlags holds ansible-specific CLI flags extracted from the command.
type ansibleCLIFlags struct {
	Playbook  string
	Inventory string
}

// getAnsibleFlags extracts ansible-specific flags from the command.
func getAnsibleFlags(cmd *cobra.Command) ansibleCLIFlags {
	cliFlags := ansibleCLIFlags{}

	if playbookFlag := cmd.Flag("playbook"); playbookFlag != nil {
		cliFlags.Playbook = playbookFlag.Value.String()
	}

	if inventoryFlag := cmd.Flag("inventory"); inventoryFlag != nil {
		cliFlags.Inventory = inventoryFlag.Value.String()
	}

	return cliFlags
}

// processArgs processes command arguments to extract component and additional args.
func processArgs(args []string) (component string, additionalArgs []string) {
	if len(args) > 0 {
		component = args[0]
		if len(args) > 1 {
			additionalArgs = args[1:]
		}
	}
	return component, additionalArgs
}

// initConfigAndStacksInfo initializes a ConfigAndStacksInfo for ansible command execution.
func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)

	// Set component type.
	info.ComponentType = cfg.AnsibleComponentType

	// Set subcommand.
	info.SubCommand = subCommand
	info.CliArgs = []string{"ansible", subCommand}

	// Process positional arguments.
	component, additionalArgs := processArgs(args)
	if component != "" {
		info.ComponentFromArg = component
	}
	info.AdditionalArgsAndFlags = additionalArgs

	return info
}
