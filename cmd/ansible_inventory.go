// https://docs.ansible.com/ansible/latest/cli/ansible-inventory.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_inventory_usage.md
var ansibleInventoryUsageMarkdown string

// Command: `atmos ansible inventory`.
var (
	ansibleInventoryShort = "Display or dump the configured inventory as Ansible sees it."
	ansibleInventoryLong  = `This command displays or dumps the configured inventory as Ansible sees it.

Example usage:
  atmos ansible inventory <component> --stack <stack> [options]
  atmos ansible inventory <component> --stack <stack> --list [options]
  atmos ansible inventory <component> -s <stack> --host <hostname> [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-inventory.html
`
)

// ansibleInventoryCmd represents the `atmos ansible inventory` command.
var ansibleInventoryCmd = &cobra.Command{
	Use:                "inventory",
	Aliases:            []string{},
	Short:              ansibleInventoryShort,
	Long:               ansibleInventoryLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "inventory", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleInventoryCmd)
}
