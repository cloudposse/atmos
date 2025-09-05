// https://docs.ansible.com/ansible/latest/cli/ansible-inventory.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_inventory_usage.md
var ansibleInventoryUsageMarkdown string

// ansibleInventoryCmd represents the `atmos ansible inventory` command.
var ansibleInventoryCmd = &cobra.Command{
	Use:                "inventory",
	Aliases:            []string{},
	Short:              "Display or dump the configured inventory as Ansible sees it.",
	Long:               "This command displays or dumps the configured inventory as Ansible sees it.",
	Example:            ansibleInventoryUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "inventory", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleInventoryCmd)
}
