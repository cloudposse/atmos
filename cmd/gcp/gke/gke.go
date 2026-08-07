package gke

import "github.com/spf13/cobra"

// GkeCmd executes gcp gke commands.
var GkeCmd = &cobra.Command{
	Use:   "gke",
	Short: "Manage GKE authentication",
	Long:  "Generate short-lived Google Cloud credentials for GKE through Atmos Auth.",
	Args:  cobra.NoArgs,
}
