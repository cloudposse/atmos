package helmfile

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/helmfile/generate"
	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
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
	addStackCompletion(helmfileCmd)

	// Add generate subcommand from the generate subpackage.
	helmfileCmd.AddCommand(generate.GenerateCmd)

	// Register this command with the registry.
	internal.Register(&HelmfileCommandProvider{})
}

// helmfileRun is the shared execution function for all helmfile subcommands.
func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
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
	return e.ExecuteHelmfile(info)
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
