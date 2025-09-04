package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

//go:embed markdown/atmos_ansible_usage.md
var ansibleUsageMarkdown string

// ansibleCmd represents the base command for all Ansible sub-commands.
var ansibleCmd = &cobra.Command{
	Use:                "ansible",
	Aliases:            []string{"as"},
	Short:              "Manage ansible playbooks for infrastructure automation",
	Long:               `Run Ansible commands for configuration management and infrastructure automation.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	ansibleCmd.DisableFlagParsing = true
	ansibleCmd.PersistentFlags().Bool("", false, doubleDashHint)
	ansibleCmd.PersistentFlags().StringP("playbook", "p", "", "Ansible playbook for configuration management")
	ansibleCmd.PersistentFlags().StringP("inventory", "i", "", "Ansible inventory file or directory")

	AddStackCompletion(ansibleCmd)
	RootCmd.AddCommand(ansibleCmd)
}

func ansibleRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info := getConfigAndStacksInfo("ansible", cmd, diffArgs)
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

	return e.ExecuteAnsible(&info, &ansibleFlags)
}
