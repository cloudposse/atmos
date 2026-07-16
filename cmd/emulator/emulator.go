package emulator

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/component"
	_ "github.com/cloudposse/atmos/pkg/component/emulator" // register the emulator component provider.
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

// emulatorParser handles flag parsing for shared emulator flags inherited by all
// emulator subcommands.
var emulatorParser *flags.StandardParser

// upParser and resetParser handle the subcommand-specific flags: `--ephemeral`
// on `up` (run a throwaway instance, no persistence) and `--force` on `reset`
// (wipe persisted state without confirmation).
var (
	upParser    *flags.StandardParser
	resetParser *flags.StandardParser
)

const (
	// The flagEphemeral is the `up` flag that runs an emulator without persistence.
	flagEphemeral = "ephemeral"
	// The flagForce is the `reset` flag that skips the confirmation prompt.
	flagForce = "force"
	// The flagRuntime bypasses project configuration for list/ps diagnostics.
	flagRuntime = "runtime"
)

// emulatorCmd is the base command for all emulator subcommands.
var emulatorCmd = &cobra.Command{
	Use:     "emulator",
	Aliases: []string{"emu"},
	Short:   "Manage local emulators for AWS, GCP, Azure, Kubernetes, Vault, and OCI registries",
	Long: `Start, stop, and operate cloud-API emulator components.

An emulator component is a stack-scoped, long-running container that stands in for
a cloud API (AWS, GCP, Azure), Kubernetes, a backing service (Vault), or an
OCI/Docker registry during local development and testing. It outlives the atmos
process and is discovered by labels derived from the canonical component instance address.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
}

func init() {
	emulatorParser = flags.NewStandardParser(
		WithEmulatorFlags(),
		flags.WithCompletionPrompt("stack", "Choose a stack", stackFlagCompletion),
	)
	emulatorParser.Registry().SetCompletionFunc("stack", stackFlagCompletion)
	emulatorParser.RegisterPersistentFlags(emulatorCmd)
	if err := emulatorParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Only `exec` accepts arbitrary flags after "--" to pass through to the
	// container; whitelisting unknown flags elsewhere would mask typos.
	execCmd.FParseErrWhitelist = struct{ UnknownFlags bool }{UnknownFlags: true}

	upParser = flags.NewStandardParser(
		flags.WithBoolFlag(flagEphemeral, "", false, "Run without persisting state; data is discarded on `down` (persistence is enabled by default)"),
		flags.WithEnvVars(flagEphemeral, "ATMOS_EMULATOR_EPHEMERAL"),
	)
	upParser.RegisterFlags(upCmd)
	if err := upParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	resetParser = flags.NewStandardParser(
		flags.WithBoolFlag(flagForce, "f", false, "Wipe persisted state without prompting for confirmation"),
		flags.WithEnvVars(flagForce, "ATMOS_EMULATOR_RESET_FORCE"),
	)
	resetParser.RegisterFlags(resetCmd)
	if err := resetParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	emulatorCmd.AddCommand(upCmd, downCmd, resetCmd, psCmd, listCmd, logsCmd, execCmd)

	RegisterEmulatorCompletions(emulatorCmd)
	internal.Register(&EmulatorCommandProvider{})
}

// EmulatorCommandProvider implements the CommandProvider interface.
type EmulatorCommandProvider struct{}

// GetCommand returns the emulator command.
func (c *EmulatorCommandProvider) GetCommand() *cobra.Command { return emulatorCmd }

// GetName returns the command name.
func (c *EmulatorCommandProvider) GetName() string { return "emulator" }

// GetGroup returns the command group for help organization.
func (c *EmulatorCommandProvider) GetGroup() string { return "Core Stack Commands" }

// GetAliases returns command aliases.
func (c *EmulatorCommandProvider) GetAliases() []internal.CommandAlias { return nil }

// GetFlagsBuilder returns the flags builder for this command.
func (c *EmulatorCommandProvider) GetFlagsBuilder() flags.Builder { return nil }

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (c *EmulatorCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (c *EmulatorCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (c *EmulatorCommandProvider) IsExperimental() bool { return true }

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
	// Prefer the parsed `--stack`/`-s` flag value (reliable on the subcommand,
	// matching completions.go); fall back to viper for the ATMOS_STACK env var.
	if stackFlag := cmd.Flag("stack"); stackFlag != nil && stackFlag.Value.String() != "" {
		info.Stack = stackFlag.Value.String()
	} else if stack := v.GetString("stack"); stack != "" {
		info.Stack = stack
	}
	if dryRunFlag := cmd.Flag("dry-run"); dryRunFlag != nil && dryRunFlag.Value.String() == "true" {
		info.DryRun = true
	}
	return info
}

// initConfigAndStacksInfo builds the execution info for an emulator subcommand,
// splitting the component (positional, before "--") from pass-through args
// (after "--", e.g. the command for `exec`).
func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)
	info.ComponentType = cfg.EmulatorComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{"emulator", subCommand}

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

// runVerb is the shared dispatch for all emulator subcommands: it builds the
// execution info and delegates to the registered emulator component provider.
func runVerb(cmd *cobra.Command, subCommand string, args []string) error {
	if err := emulatorParser.BindFlagsToViper(cmd, viper.GetViper()); err != nil {
		return err
	}
	// Cobra leaves pass-through arguments after `--` in args. Parse only the
	// command-owned portion so flags for the command executed by `emulator exec`
	// (for example, `kubectl -n demo`) are never interpreted as Atmos flags.
	positional, _ := flags.SplitArgsAtDash(cmd, args)
	if requiresStack(subCommand) {
		parsed, err := emulatorParser.Parse(context.Background(), positional)
		if err != nil {
			return err
		}
		// Interactive selections only live in the parsed result. Carry the
		// value forward explicitly rather than mutating global Viper state.
		if parsed.Stack != "" {
			if stackFlag := cmd.Flag("stack"); stackFlag != nil {
				if err := stackFlag.Value.Set(parsed.Stack); err != nil {
					return err
				}
			}
		}
	}
	if parser, ok := componentPromptParsers[cmd]; ok {
		parsed, err := parser.Parse(context.Background(), positional)
		if err != nil {
			return err
		}
		if len(positional) == 0 && len(parsed.GetPositionalArgs()) > 0 {
			args = append([]string{parsed.Component}, args...)
		}
	}
	info := initConfigAndStacksInfo(cmd, subCommand, args)
	provider := component.MustGetProvider(cfg.EmulatorComponentType)
	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.EmulatorComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             "emulator",
		SubCommand:          subCommand,
		ConfigAndStacksInfo: info,
		Args:                info.AdditionalArgsAndFlags,
		Flags:               verbFlags(cmd),
	})
}

func requiresStack(subCommand string) bool {
	switch subCommand {
	case "up", "down", "reset", "logs", "exec":
		return true
	default:
		return false
	}
}

// verbFlags reads the subcommand-specific flags (`--ephemeral` on up, `--force`
// on reset) into a map for the component executor, honoring flag > env > default
// precedence via Viper. Only flags registered on the given command are read.
func verbFlags(cmd *cobra.Command) map[string]any {
	v := viper.GetViper()
	flagsMap := map[string]any{}
	if cmd.Flags().Lookup(flagEphemeral) != nil {
		if err := upParser.BindFlagsToViper(cmd, v); err == nil {
			flagsMap[flagEphemeral] = v.GetBool(flagEphemeral)
		}
	}
	if cmd.Flags().Lookup(flagForce) != nil {
		if err := resetParser.BindFlagsToViper(cmd, v); err == nil {
			flagsMap[flagForce] = v.GetBool(flagForce)
		}
	}
	if runtimeFlag := cmd.Flag(flagRuntime); runtimeFlag != nil && runtimeFlag.Value.String() == "true" {
		flagsMap[flagRuntime] = true
	}
	return flagsMap
}
