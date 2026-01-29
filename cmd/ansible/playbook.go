// Ansible playbook CLI docs: https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html.

package ansible

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// playbookCmd represents the `atmos ansible playbook` command.
var playbookCmd = &cobra.Command{
	Use:     "playbook",
	Aliases: []string{"pb"},
	Short:   "Run Ansible playbooks for configuration management.",
	Long: `This command runs an Ansible playbook, applying configuration changes to target hosts.

Example usage:
  atmos ansible playbook <component> --stack <stack> [options]
  atmos ansible playbook <component> --stack <stack> --playbook <playbook.yml> [options]
  atmos ansible playbook <component> -s <stack> -p <playbook.yml> [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-playbook.html
`,
	// FParseErrWhitelist allows unknown flags to pass through to ansible-playbook.
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE:               runPlaybook,
}

// runPlaybook executes the ansible playbook command.
func runPlaybook(cmd *cobra.Command, args []string) error {
	// Initialize config and stacks info.
	info := initConfigAndStacksInfo(cmd, "playbook", args)

	// Get ansible-specific flags.
	ansibleFlags := getAnsibleFlags(cmd)

	// Execute ansible playbook.
	return e.ExecuteAnsible(&info, &ansibleFlags)
}
