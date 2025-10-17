package version

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

var showFormat string

var showCmd = &cobra.Command{
	Use:   "show <version>",
	Short: "Show details for a specific Atmos release",
	Long:  `Display detailed information about a specific Atmos release including release notes and download links.`,
	Example: `  # Show details for a specific version
  atmos version show v1.95.0

  # Show details for the latest release
  atmos version show latest

  # Output as JSON
  atmos version show v1.95.0 --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return e.ExecuteVersionShow(atmosConfigPtr, args[0], showFormat)
	},
}

func init() {
	showCmd.Flags().StringVar(&showFormat, "format", "text", "Output format: text, json, yaml")
	versionCmd.AddCommand(showCmd)
}
