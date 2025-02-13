package cmd

import (
	_ "embed"

	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

//go:embed markdown/about.md
var aboutMarkdown string

// aboutCmd represents the about command
var aboutCmd = &cobra.Command{
	Use:   "about",
	Short: "Learn about Atmos",
	Long:  `Display information about Atmos, its features, and benefits.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		utils.PrintfMarkdown("%s", aboutMarkdown)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(aboutCmd)
}
