// https://docs.ansible.com/ansible/latest/cli/ansible-vault.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_vault_usage.md
var ansibleVaultUsageMarkdown string

// ansibleVaultCmd represents the `atmos ansible vault` command.
var ansibleVaultCmd = &cobra.Command{
	Use:                "vault",
	Aliases:            []string{},
	Short:              "Encrypt and decrypt files within Ansible infrastructure.",
	Long:               "This command allows you to create, edit, encrypt, decrypt, and view encrypted files using Ansible Vault.",
	Example:            ansibleVaultUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "vault", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleVaultCmd)
}
