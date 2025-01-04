package cmd

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

//go:embed markdown/about.md
var aboutMarkdown string

// aboutCmd represents the about command
var aboutCmd = &cobra.Command{
	Use:   "about",
	Short: "Learn about Atmos",
	Long:  `Display information about Atmos, its features, and benefits.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return fmt.Errorf("failed to create markdown renderer: %w", err)
		}

		out, err := renderer.Render(aboutMarkdown)
		if err != nil {
			return fmt.Errorf("failed to render about documentation: %w", err)
		}

		fmt.Fprint(os.Stdout, out)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(aboutCmd)
}
