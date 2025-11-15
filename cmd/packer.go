package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// packerCmd represents the base command for all Packer sub-commands.
var packerCmd = &cobra.Command{
	Use:                "packer",
	Aliases:            []string{"pk"},
	Short:              "Manage packer-based machine images for multiple platforms",
	Long:               `Run Packer commands for creating identical machine images for multiple platforms from a single source configuration.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	packerCmd.DisableFlagParsing = true
	packerCmd.PersistentFlags().Bool("", false, doubleDashHint)
	packerCmd.PersistentFlags().StringP("template", "t", "", "Packer template for building machine images")
	packerCmd.PersistentFlags().StringP("query", "q", "", "YQ expression to read an output from the Packer manifest")

	AddStackCompletion(packerCmd)
	RootCmd.AddCommand(packerCmd)
}

func packerRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)
	// Enable heatmap tracking if --heatmap flag is present in os.Args
	// (needed because flag parsing is disabled for packer commands).
	enableHeatmapIfRequested()
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info, err := getConfigAndStacksInfo("packer", cmd, diffArgs)
	if err != nil {
		return err
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
