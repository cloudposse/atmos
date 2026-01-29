package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// ansibleCmd represents the base command for all Ansible sub-commands.
var ansibleCmd = &cobra.Command{
	Use:                "ansible",
	Aliases:            []string{"an"},
	Short:              "Manage ansible-based automation for infrastructure configuration",
	Long:               `Run Ansible commands for automating infrastructure configuration, application deployment, and orchestration.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	ansibleCmd.DisableFlagParsing = true
	ansibleCmd.PersistentFlags().Bool("", false, doubleDashHint)
	ansibleCmd.PersistentFlags().StringP("playbook", "p", "", "Ansible playbook to execute")
	ansibleCmd.PersistentFlags().StringP("inventory", "i", "", "Ansible inventory source")

	AddStackCompletion(ansibleCmd)
	RootCmd.AddCommand(ansibleCmd)
}

func ansibleRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)
	// Enable heatmap tracking if --heatmap flag is present in os.Args
	// (needed because flag parsing is disabled for ansible commands).
	enableHeatmapIfRequested()
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info, err := getConfigAndStacksInfo("ansible", cmd, diffArgs)
	if err != nil {
		return err
	}
	info.CliArgs = []string{"ansible", commandName}

	flags := cmd.Flags()

	playbook, err := flags.GetString("playbook")
	if err != nil {
		return err
	}

	inventory, err := flags.GetString("inventory")
	if err != nil {
		return err
	}

	ansibleFlags := e.AnsibleFlags{
		Playbook:  playbook,
		Inventory: inventory,
	}

	if commandName == "version" {
		_, err := e.ExecuteAnsibleVersion(&info)
		return err
	}

	return e.ExecuteAnsible(&info, &ansibleFlags)
}
