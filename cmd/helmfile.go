package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

// helmfileParser handles flag parsing for helmfile commands.
var helmfileParser *flags.HelmfileParser

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:     "helmfile",
	Aliases: []string{"hf"},
	Short:   "Manage Helmfile-based Kubernetes deployments",
	Long:    `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	// Allow arbitrary args so subcommands can accept positional arguments
	Args: cobra.ArbitraryArgs,
}

func init() {
	// Create parser with Helmfile flags.
	// Returns strongly-typed HelmfileInterpreter.
	helmfileParser = flags.NewHelmfileParser()

	// Register flags with Cobra.
	helmfileParser.RegisterFlags(helmfileCmd)
	_ = helmfileParser.BindToViper(viper.GetViper())

	AddStackCompletion(helmfileCmd)
	RootCmd.AddCommand(helmfileCmd)
}

func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)

	// Parse args with flagparser
	ctx := cmd.Context()
	opts, err := helmfileParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Build args array from interpreter for getConfigAndStacksInfo
	// PositionalArgs contains [component] for helmfile commands
	fullArgs := append([]string{commandName}, opts.GetPositionalArgs()...)
	fullArgs = append(fullArgs, opts.GetPassThroughArgs()...)

	info := getConfigAndStacksInfo("helmfile", cmd, fullArgs)

	// Use strongly-typed interpreter fields - no runtime assertions!
	info.Stack = opts.Stack
	info.Identity = opts.Identity.Value()
	info.DryRun = opts.DryRun

	// Handle --identity flag for interactive selection.
	// If identity is "__SELECT__", prompt for interactive selection.
	if opts.Identity.IsInteractiveSelector() {
		handleInteractiveIdentitySelection(&info)
	}

	info.CliArgs = []string{"helmfile", commandName}
	err = e.ExecuteHelmfile(info)
	return err
}
