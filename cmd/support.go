package cmd

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
)

//go:embed markdown/support.md
var supportMarkdown string

// supportCmd represents the support command
var supportCmd = &cobra.Command{
	Use:   "support",
	Short: "Show Atmos support options",
	Long:  `Display information about Atmos support options, including community resources and paid support.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
		if err != nil {
			return fmt.Errorf("failed to create markdown renderer: %w", err)
		}

		out, err := renderer.Render(supportMarkdown)
		if err != nil {
			return fmt.Errorf("failed to render support documentation: %w", err)
		}

		fmt.Fprint(os.Stdout, out)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(supportCmd)
}
