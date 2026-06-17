package helmfile

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/helmfile/generate"
	"github.com/cloudposse/atmos/cmd/helmfile/source"
	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// doubleDashHint is displayed in help output.
const doubleDashHint = "Use double dashes to separate Atmos-specific options from native arguments and flags for the command."

// helmfileCmd represents the base command for all helmfile sub-commands.
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Manage Helmfile-based Kubernetes deployments",
	Long:               `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	// Note: We use FParseErrWhitelist.UnknownFlags=true (set in command definition)
	// instead of DisableFlagParsing=true to allow unknown flags through to helmfile
	// while still enabling Cobra to parse known Atmos flags and display proper help.
	helmfileCmd.PersistentFlags().Bool("", false, doubleDashHint)
	helmfileCmd.PersistentFlags().Bool("ci", false, "Enable CI mode for automated pipelines (writes job summary).")
	addStackCompletion(helmfileCmd)

	// Add generate subcommand from the generate subpackage.
	helmfileCmd.AddCommand(generate.GenerateCmd)

	// Add source subcommand from the source subpackage.
	helmfileCmd.AddCommand(source.GetSourceCommand())

	// Register this command with the registry.
	internal.Register(&HelmfileCommandProvider{})
}

// helmfileRun is the shared execution function for all helmfile subcommands.
func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
	argsCI, args := stripCIFlag(args)
	// Check if help was requested and display it.
	if handleHelpRequest(cmd, args) {
		return nil
	}
	// Enable heatmap tracking if --heatmap flag is present in os.Args
	// (needed because flag parsing is disabled for helmfile commands).
	enableHeatmapIfRequested()
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info, err := getConfigAndStacksInfo("helmfile", cmd, diffArgs)
	if err != nil {
		return err
	}
	info.CliArgs = []string{"helmfile", commandName}

	hookCalled := false
	forceCIMode := helmfileCIModeEnabled(cmd, argsCI)
	info.PerComponentHook = func(hookInfo *schema.ConfigAndStacksInfo, output string, execErr error) {
		hookCalled = true
		runHelmfileCIHook(commandName, hookInfo, output, execErr, forceCIMode)
	}

	err = e.ExecuteHelmfile(info)
	if err != nil && !hookCalled {
		runHelmfileCIHook(commandName, &info, "", err, forceCIMode)
	}
	return err
}

func runHelmfileCIHook(commandName string, info *schema.ConfigAndStacksInfo, output string, execErr error, forceCIMode bool) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Warn("CI hook config init failed", "component", info.ComponentFromArg, "error", err)
		return
	}

	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:        helmfileAfterEvent(commandName),
		AtmosConfig:  &atmosConfig,
		Info:         info,
		Output:       output,
		ForceCIMode:  forceCIMode,
		CommandError: execErr,
		ExitCode:     errUtils.GetExitCode(execErr),
	}); err != nil {
		log.Warn("CI hook execution failed", "component", info.ComponentFromArg, "error", err)
	}
}

func helmfileAfterEvent(commandName string) h.HookEvent {
	switch commandName {
	case "template":
		return h.AfterHelmfileTemplate
	case "diff":
		return h.AfterHelmfileDiff
	case "apply":
		return h.AfterHelmfileApply
	case "sync":
		return h.AfterHelmfileSync
	case "deploy":
		return h.AfterHelmfileDeploy
	case "destroy":
		return h.AfterHelmfileDestroy
	default:
		return h.HookEvent("after.helmfile." + commandName)
	}
}

func helmfileCIModeEnabled(cmd *cobra.Command, argsCI bool) bool {
	if argsCI {
		return true
	}
	if cmd != nil {
		if value, err := cmd.Flags().GetBool("ci"); err == nil && value {
			return true
		}
		if value, err := cmd.InheritedFlags().GetBool("ci"); err == nil && value {
			return true
		}
	}
	return ciEnvEnabled("ATMOS_CI") || ciEnvEnabled("CI")
}

func stripCIFlag(args []string) (bool, []string) {
	result := make([]string, 0, len(args))
	enabled := false
	for _, arg := range args {
		switch {
		case arg == "--ci":
			enabled = true
		case strings.HasPrefix(arg, "--ci="):
			value := strings.TrimPrefix(arg, "--ci=")
			enabled = ciValueEnabled(value)
		default:
			result = append(result, arg)
		}
	}
	return enabled, result
}

func ciEnvEnabled(key string) bool {
	return ciValueEnabled(os.Getenv(key))
}

func ciValueEnabled(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized != "" && normalized != "false" && normalized != "0" && normalized != "no"
}

// HelmfileCommandProvider implements the CommandProvider interface.
type HelmfileCommandProvider struct{}

// GetCommand returns the helmfile command.
func (h *HelmfileCommandProvider) GetCommand() *cobra.Command {
	return helmfileCmd
}

// GetName returns the command name.
func (h *HelmfileCommandProvider) GetName() string {
	return "helmfile"
}

// GetGroup returns the command group for help organization.
func (h *HelmfileCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

// GetAliases returns command aliases.
func (h *HelmfileCommandProvider) GetAliases() []internal.CommandAlias {
	return nil // No aliases for helmfile command.
}

// GetFlagsBuilder returns the flags builder for this command.
func (h *HelmfileCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil // Helmfile uses pass-through flag parsing.
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (h *HelmfileCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil // Helmfile command has subcommands, not positional args.
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (h *HelmfileCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil // Helmfile uses pass-through flag parsing.
}

// IsExperimental returns whether this command is experimental.
func (h *HelmfileCommandProvider) IsExperimental() bool {
	return false
}
