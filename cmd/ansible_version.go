package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_version_usage.md
var ansibleVersionUsageMarkdown string

// Command: `atmos ansible version`.
var (
	ansibleVersionShort = "Show the version of ansible command."
	ansibleVersionLong  = `This command shows the version of ansible command.

Example usage:
  atmos ansible version
`
)

// ansibleVersionCmd represents the `atmos ansible version` command.
var ansibleVersionCmd = &cobra.Command{
	Use:                "version",
	Aliases:            []string{},
	Short:              ansibleVersionShort,
	Long:               ansibleVersionLong,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "version", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleVersionCmd)
}
