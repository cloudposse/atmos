package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flagparser"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// packerParser handles flag parsing for packer commands.
var packerParser *flagparser.PassThroughFlagParser

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
	// This replaces DisableFlagParsing and manual flag handling.
	packerParser = flagparser.NewPassThroughFlagParser(
		flagparser.WithPackerFlags(),
	)

	// Register flags with Cobra.
	packerParser.RegisterFlags(packerCmd)
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
	parsedConfig, err := packerParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Build args array from ParsedConfig for getConfigAndStacksInfo
	fullArgs := []string{commandName}
	if parsedConfig.ComponentName != "" {
		fullArgs = append(fullArgs, parsedConfig.ComponentName)
	}
	fullArgs = append(fullArgs, parsedConfig.PassThroughArgs...)

	info := getConfigAndStacksInfo("packer", cmd, fullArgs)
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
