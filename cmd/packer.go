package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// packerParser handles flag parsing for packer commands.
var packerParser *flags.PackerParser

// packerCmd represents the base command for all Packer sub-commands.
var packerCmd = &cobra.Command{
	Use:     "packer",
	Aliases: []string{"pk"},
	Short:   "Manage packer-based machine images for multiple platforms",
	Long:    `Run Packer commands for creating identical machine images for multiple platforms from a single source configuration.`,
	Args:    cobra.NoArgs,
}

func init() {
	// Create parser with Packer flags.
	// Returns strongly-typed PackerOptions.
	packerParser = flags.NewPackerParser()

	// Register flags as persistent (inherited by subcommands).
	packerParser.RegisterPersistentFlags(packerCmd)
	_ = packerParser.BindToViper(viper.GetViper())

	// Packer-specific flags
	packerCmd.PersistentFlags().StringP("template", "t", "", "Packer template for building machine images")
	packerCmd.PersistentFlags().StringP("query", "q", "", "YQ expression to read an output from the Packer manifest")

	AddStackCompletion(packerCmd)
	RootCmd.AddCommand(packerCmd)
}

func packerRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)

	// Parse args with flagparser
	ctx := cmd.Context()
	opts, err := packerParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Build args array from interpreter for getConfigAndStacksInfo
	// PositionalArgs contains [component] for packer commands
	fullArgs := append([]string{commandName}, opts.GetPositionalArgs()...)
	fullArgs = append(fullArgs, opts.GetPassThroughArgs()...)

	info := getConfigAndStacksInfo("packer", cmd, fullArgs)

	// Use strongly-typed interpreter fields - no runtime assertions!
	info.Stack = opts.Stack
	info.Identity = opts.Identity.Value()
	info.DryRun = opts.DryRun

	// Handle --identity flag for interactive selection.
	// If identity is "__SELECT__", prompt for interactive selection.
	if opts.Identity.IsInteractiveSelector() {
		handleInteractiveIdentitySelection(&info)
	}

	info.CliArgs = []string{"packer", commandName}

	flags := cmd.Flags()

	template, err := flags.GetString("template")
	if err != nil {
		return err
	}

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	packerFlags := e.PackerFlags{
		Template: template,
		Query:    query,
	}

	if commandName == "output" {
		d, err := e.ExecutePackerOutput(&info, &packerFlags)
		if err != nil {
			return err
		}
		err = u.PrintAsYAML(&atmosConfig, d)
		return err
	}

	return e.ExecutePacker(&info, &packerFlags)
}
