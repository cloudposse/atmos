package gke

import (
	_ "embed"

	"github.com/spf13/cobra"
)

//go:embed markdown/atmos_gcp_gke_usage.md
var gkeUsageMarkdown string

// GkeCmd executes gcp gke commands.
var GkeCmd = &cobra.Command{
	Use:     "gke",
	Short:   "Manage GKE authentication",
	Long:    "Generate short-lived Google Cloud credentials for GKE through Atmos Auth.",
	Example: gkeUsageMarkdown,
	Args:    cobra.NoArgs,
}
