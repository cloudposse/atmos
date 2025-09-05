package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_ansible_version_usage.md
var ansibleVersionUsageMarkdown string

// ansibleVersionCmd represents the `atmos ansible version` command.
var ansibleVersionCmd = &cobra.Command{
	Use:                "version",
	Aliases:            []string{},
	Short:              "Show the version of ansible command.",
	Long:               "This command shows the version of ansible command.",
	Example:            ansibleVersionUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return ansibleRun(cmd, "version", args)
	},
}

func init() {
	ansibleCmd.AddCommand(ansibleVersionCmd)
}
