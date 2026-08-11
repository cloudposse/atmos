package container

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// containerParser handles flag parsing for shared container flags inherited by
// all container subcommands.
var containerParser *flags.StandardParser

// containerCmd is the base command for all container subcommands.
var containerCmd = &cobra.Command{
	Use:     "container",
	Aliases: []string{"c"},
	Short:   "Manage persistent, stack-scoped container components",
	Long: `Build, run, and operate container components.

A container component is a stack-scoped, Atmos-native service: one component is one
container. Atmos owns the image artifact (build/push/pull) and an optional long-running
named container lifecycle (up/ps/logs/exec/restart/stop/rm/down), discovered by labels
derived from the canonical component instance address — not from local state files.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
}

func init() {
	containerParser = flags.NewStandardParser(WithContainerFlags())
	containerParser.Registry().SetCompletionFunc("stack", stackFlagCompletion)
	containerParser.RegisterPersistentFlags(containerCmd)
	if err := containerParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Only `exec` accepts arbitrary flags after "--" to pass through to the
	// container. Whitelisting unknown flags on the other verbs would mask typos,
	// so scope it to execCmd alone (not all verbs).
	execCmd.FParseErrWhitelist = struct{ UnknownFlags bool }{UnknownFlags: true}

	containerCmd.AddCommand(
		listCmd,
		buildCmd, pushCmd, pullCmd, runCmd, upCmd, startCmd, psCmd,
		logsCmd, execCmd, attachCmd, restartCmd, stopCmd, rmCmd, downCmd,
	)

	RegisterContainerCompletions(containerCmd)
	internal.Register(&ContainerCommandProvider{})
}

// ContainerCommandProvider implements the CommandProvider interface.
type ContainerCommandProvider struct{}

// GetCommand returns the container command.
func (c *ContainerCommandProvider) GetCommand() *cobra.Command { return containerCmd }

// GetName returns the command name.
func (c *ContainerCommandProvider) GetName() string { return "container" }

// GetGroup returns the command group for help organization.
func (c *ContainerCommandProvider) GetGroup() string { return "Core Stack Commands" }

// GetAliases returns command aliases.
func (c *ContainerCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// GetFlagsBuilder returns the flags builder for this command.
func (c *ContainerCommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (c *ContainerCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (c *ContainerCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (c *ContainerCommandProvider) IsExperimental() bool { return false }

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo with global flags populated.
func buildConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)

	info := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		Identity:                cfg.NormalizeIdentityValue(globalFlags.Identity.Value()),
		ProfilesFromArg:         globalFlags.Profile,
	}

	// Resolve the stack via viper so the full precedence chain is honored
	// (flag > ATMOS_STACK env > config), not just the directly-set Cobra flag.
	if stack := v.GetString("stack"); stack != "" {
		info.Stack = stack
	}
	if dryRunFlag := cmd.Flag("dry-run"); dryRunFlag != nil && dryRunFlag.Value.String() == "true" {
		info.DryRun = true
	}

	// `--all`/`--tags`/`--labels` are registered only on the bulk-capable
	// lifecycle verbs, so read them directly from the executing command's flags
	// (nil-safe for the other verbs).
	if allFlag := cmd.Flag("all"); allFlag != nil && allFlag.Value.String() == "true" {
		info.All = true
	}
	if tagsSlice, err := cmd.Flags().GetStringSlice("tags"); err == nil {
		info.Tags = tags.ParseTagsFlag(strings.Join(tagsSlice, ","))
	}
	if labelsFlag := cmd.Flag("labels"); labelsFlag != nil {
		// Error ignored: the wrapped Args validator in buildVerbCmd already
		// rejected a malformed --labels value before RunE.
		info.Labels, _ = tags.ParseLabelsFlag(labelsFlag.Value.String())
	}

	return info
}

// initConfigAndStacksInfo builds the execution info for a container subcommand,
// splitting the component (positional, before "--") from pass-through args
// (after "--", e.g. the command for `exec`).
func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)
	info.ComponentType = cfg.ContainerComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{"container", subCommand}

	positional, separated := flags.SplitArgsAtDash(cmd, args)
	if len(positional) > 0 {
		info.ComponentFromArg = positional[0]
	}
	if len(positional) > 1 {
		info.AdditionalArgsAndFlags = positional[1:]
	}
	info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, separated...)

	return info
}

// runVerb is the shared dispatch for all container subcommands: it builds the
// execution info and delegates to the registered container component provider.
func runVerb(cmd *cobra.Command, subCommand string, args []string) error {
	// Rebind the executing command's flags (including the inherited --stack) to
	// Viper so the full precedence chain (flag > ATMOS_STACK env > config) is
	// honored. The init-time BindToViper binds the parser's own flag set, not the
	// flags Cobra actually parses into, so without this per-execution rebind the
	// --stack flag value is silently dropped (only ATMOS_STACK would be read).
	if err := containerParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return err
	}
	if parser, ok := componentPromptParsers[cmd]; ok {
		parsed, err := parser.Parse(context.Background(), args)
		if err != nil {
			return err
		}
		positional, _ := flags.SplitArgsAtDash(cmd, args)
		if len(positional) == 0 && len(parsed.GetPositionalArgs()) > 0 {
			args = append([]string{parsed.Component}, args...)
		}
	}

	info := initConfigAndStacksInfo(cmd, subCommand, args)
	provider := component.MustGetProvider(cfg.ContainerComponentType)
	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.ContainerComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             "container",
		SubCommand:          subCommand,
		ConfigAndStacksInfo: info,
		// Args carries the pass-through command (after "--"), used by `exec`.
		Args: info.AdditionalArgsAndFlags,
		// Flags carries verb-specific flags (e.g. logs --follow/--tail) when present.
		Flags: verbFlags(cmd),
	})
}

// verbFlags collects verb-specific flag values (nil-safe — only the flags
// registered on the executing command are present) for the component provider.
func verbFlags(cmd *cobra.Command) map[string]any {
	flagValues := map[string]any{}
	if f := cmd.Flag("follow"); f != nil {
		flagValues["follow"] = f.Value.String() == "true"
	}
	if f := cmd.Flag("tail"); f != nil {
		flagValues["tail"] = f.Value.String()
	}
	return flagValues
}
