package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/utils"
)

//go:embed markdown/support.md
var supportMarkdown string

// supportCmd represents the support command.
var supportCmd = &cobra.Command{
	Use:                "support",
	Short:              "Show Atmos support options",
	Long:               `Display information about Atmos support options, including community resources and paid support.`,
	Args:               cobra.NoArgs,
	DisableSuggestions: true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintfMarkdown("%s", supportMarkdown)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(supportCmd)
}
