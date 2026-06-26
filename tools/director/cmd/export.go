package cmd

import (
	"github.com/spf13/cobra"
)

func exportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export demo data and metadata",
		Long: `Export commands for generating gallery data, metadata, and manifests.

The export command provides subcommands for exporting VHS demo metadata
in various formats suitable for integration with website components.`,
		Example: `
# Export gallery manifest JSON
director export manifest > website/src/data/demos.json
`,
	}

	// Add subcommands.
	cmd.AddCommand(exportManifestCmd())

	return cmd
}
