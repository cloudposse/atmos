// https://docs.ansible.com/ansible/latest/cli/ansible-vault.html

package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_vault_usage.md
var ansibleVaultUsageMarkdown string

// Command: `atmos ansible vault`.
var (
	ansibleVaultShort = "Encrypt and decrypt files within Ansible infrastructure."
	ansibleVaultLong  = `This command allows you to create, edit, encrypt, decrypt, and view encrypted files using Ansible Vault.

Example usage:
  atmos ansible vault <component> --stack <stack> [action] [file] [options]
  atmos ansible vault <component> --stack <stack> encrypt [file] [options]
  atmos ansible vault <component> -s <stack> decrypt [file] [options]

To see all available options, refer to https://docs.ansible.com/ansible/latest/cli/ansible-vault.html
`
)

// ansibleVaultCmd represents the `atmos ansible vault` command.
var ansibleVaultCmd = &cobra.Command{
	Use:                "vault",
	Aliases:            []string{},
	Short:              ansibleVaultShort,
	Long:               ansibleVaultLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "vault", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleVaultCmd)
}
