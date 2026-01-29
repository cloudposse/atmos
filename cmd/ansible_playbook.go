// https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos ansible playbook`.
var (
	ansiblePlaybookShort = "Run Ansible playbooks for configuration management."
	ansiblePlaybookLong  = `This command runs an Ansible playbook, applying configuration changes to target hosts.

Example usage:
  atmos ansible playbook <component> --stack <stack> [options]
  atmos ansible playbook <component> --stack <stack> --playbook <playbook.yml> [options]
  atmos ansible playbook <component> -s <stack> -p <playbook.yml> [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html
`
)

// ansiblePlaybookCmd represents the ` + "`atmos ansible playbook`" + ` command.
var ansiblePlaybookCmd = &cobra.Command{
	Use:                "playbook",
	Aliases:            []string{"pb"},
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
