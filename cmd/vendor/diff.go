package vendor

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// vendorDiffCmd shows the Git diff between two versions of a vendored component.
var vendorDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the Git diff between two versions of a vendored component",
	Long: `Show the changes between two versions (tags, branches, or commits) of a vendored
Git component, without a local checkout. Defaults --from to the component's
current pinned version and --to to the latest tag.`,
	Example: "atmos vendor diff --component vpc\natmos vendor diff -c vpc --from 1.0.0 --to 2.0.0",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.diffRunE")()

		component, _ := cmd.Flags().GetString("component")
		if component == "" {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithExplanation("The --component flag is required.").
				WithHint("Specify which component to diff, e.g. atmos vendor diff --component vpc.").
				Err()
		}

		file, err := resolveVendorFile()
		if err != nil {
			return err
		}
		files, err := vendoring.CollectManifestFiles(file)
		if err != nil {
			return err
		}

		src, _, err := vendoring.FindSource(files, component)
		if err != nil {
			return err
		}

		from, _ := cmd.Flags().GetString("from")
		if from == "" {
			from = src.Version
		}
		to, _ := cmd.Flags().GetString("to")
		fileFilter, _ := cmd.Flags().GetString("diff-file")

		diff, err := vendoring.Diff(nil, &vendoring.DiffParams{
			Source: src.Source,
			From:   from,
			To:     to,
			File:   fileFilter,
		})
		if err != nil {
			return err
		}
		return data.Writeln(diff)
	},
}

func init() {
	vendorDiffCmd.Flags().StringP("component", "c", "", "Component to diff (required)")
	vendorDiffCmd.Flags().String("from", "", "Starting ref (default: current pinned version)")
	vendorDiffCmd.Flags().String("to", "", "Ending ref (default: latest tag)")
	vendorDiffCmd.Flags().String("diff-file", "", "Restrict the diff to a single file path within the component")
	vendorDiffCmd.Flags().StringVar(&vendorFileFlag, "file", "", "Vendor manifest file (default: ./vendor.yaml)")

	vendorCmd.AddCommand(vendorDiffCmd)
}
