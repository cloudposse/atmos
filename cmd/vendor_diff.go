package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorDiffCmd executes 'vendor diff' CLI commands.
var vendorDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show Git diff between two versions of a vendored component",
	Long: `This command shows the Git diff between two versions of a vendored component from the remote repository.

The command uses Git to compare two refs (tags, branches, or commits) without requiring a local clone.
Output is colorized automatically when output is to a terminal.

Use --from and --to to specify versions, or let it default to current version vs latest.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Vendor diff doesn't require stack validation
		checkAtmosConfig()

		err := e.ExecuteVendorDiffCmd(cmd, args)
		return err
	},
}

func init() {
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "Component to diff (required)")
	_ = vendorDiffCmd.RegisterFlagCompletionFunc("component", ComponentsArgCompletion)
	vendorDiffCmd.PersistentFlags().String("from", "", "Starting version/tag/commit (defaults to current version in vendor.yaml)")
	vendorDiffCmd.PersistentFlags().String("to", "", "Ending version/tag/commit (defaults to latest)")
	vendorDiffCmd.PersistentFlags().String("file", "", "Show diff for specific file within component")
	vendorDiffCmd.PersistentFlags().IntP("context", "C", 3, "Number of context lines")
	vendorDiffCmd.PersistentFlags().Bool("unified", true, "Show unified diff format")
	vendorCmd.AddCommand(vendorDiffCmd)
}
