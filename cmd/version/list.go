package version

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

const (
	listDefaultLimit = 10
)

var (
	listLimit              int
	listOffset             int
	listSince              string
	listIncludePrereleases bool
	listFormat             string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Atmos releases",
	Long:  `List available Atmos releases from GitHub with pagination and filtering options.`,
	Example: `  # List the last 10 releases (default)
  atmos version list

  # List the last 20 releases
  atmos version list --limit 20

  # List releases starting from offset 10
  atmos version list --offset 10

  # Include pre-releases
  atmos version list --include-prereleases

  # List releases since a specific date
  atmos version list --since 2025-01-01

  # Output as JSON
  atmos version list --format json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return e.ExecuteVersionList(atmosConfigPtr, listLimit, listOffset, listSince, listIncludePrereleases, listFormat)
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", listDefaultLimit, "Maximum number of releases to display (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of releases to skip")
	listCmd.Flags().StringVar(&listSince, "since", "", "Only show releases published after this date (ISO 8601 format: YYYY-MM-DD)")
	listCmd.Flags().BoolVar(&listIncludePrereleases, "include-prereleases", false, "Include pre-release versions")
	listCmd.Flags().StringVar(&listFormat, "format", "text", "Output format: text, json, yaml")

	versionCmd.AddCommand(listCmd)
}
