package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
)

//go:embed markdown/support.md
var supportMarkdown string

// supportCmd represents the support command
var supportCmd = &cobra.Command{
	Use:                "support",
	Short:              "Show Atmos support options",
	Long:               `Display information about Atmos support options, including community resources and paid support.`,
	Args:               cobra.NoArgs,
	DisableSuggestions: true,
	SilenceUsage:       true,
	SilenceErrors:      true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Word-wrap would hard-break the long inline URLs in supportMarkdown
		// mid-domain when they don't fit the wrap width (e.g. producing
		// "https://github. com/..."); render without wrapping instead.
		return data.MarkdownNoWrapf("%s", supportMarkdown)
	},
}

func init() {
	RootCmd.AddCommand(supportCmd)
}
