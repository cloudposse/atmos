// https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_playbook_usage.md
var ansiblePlaybookUsageMarkdown string

// Command: `atmos ansible playbook`.
var (
	ansiblePlaybookShort = "Run an Ansible playbook for configuration management."
	ansiblePlaybookLong  = `This command takes an Ansible playbook and runs it against the specified inventory.

Example usage:
  atmos ansible playbook <component> --stack <stack> [options]
  atmos ansible playbook <component> --stack <stack> --playbook <playbook> [options]
  atmos ansible playbook <component> -s <stack> --p <playbook> [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html
`
)

// ansiblePlaybookCmd represents the `atmos ansible playbook` command.
var ansiblePlaybookCmd = &cobra.Command{
	Use:                "playbook",
	Aliases:            []string{},
	Short:              ansiblePlaybookShort,
	Long:               ansiblePlaybookLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "playbook", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansiblePlaybookCmd)
}
