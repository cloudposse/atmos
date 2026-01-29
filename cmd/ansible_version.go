// https://docs.ansible.com/ansible/latest/cli/ansible.html

package cmd

import (
	"github.com/spf13/cobra"
)

// Command: `atmos ansible version`.
var (
	ansibleVersionShort = "Show Ansible version information."
	ansibleVersionLong  = `This command shows the Ansible version, config file location, and module search path.

Example usage:
  atmos ansible version

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible.html
`
)

// ansibleVersionCmd represents the ` + "`atmos ansible version`" + ` command.
var ansibleVersionCmd = &cobra.Command{
	Use:   "version",
	Short: ansibleVersionShort,
	Long:  ansibleVersionLong,
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "version", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleVersionCmd)
}
