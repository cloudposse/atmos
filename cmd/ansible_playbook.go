// https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_playbook_usage.md
var ansiblePlaybookUsageMarkdown string

// ansiblePlaybookCmd represents the `atmos ansible playbook` command.
var ansiblePlaybookCmd = &cobra.Command{
	Use:                "playbook",
	Aliases:            []string{},
	Short:              "Run an Ansible playbook for configuration management.",
	Long:               "This command takes an Ansible playbook and runs it against the specified inventory.",
	Example:            ansiblePlaybookUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "playbook", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansiblePlaybookCmd)
}
