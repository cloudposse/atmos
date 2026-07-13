package vendor

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

var vendorDiffParser *flags.StandardParser

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

		v := viper.GetViper()
		if err := vendorDiffParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		component := v.GetString("component")
		if component == "" {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithExplanation("The --component flag is required.").
				WithHint("Specify which component to diff, e.g. atmos vendor diff --component vpc.").
				Err()
		}

		resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{
			VendorFile:    v.GetString("file"),
			Component:     component,
			ComponentType: v.GetString("type"),
		})
		if err != nil {
			return err
		}

		from := v.GetString("from")
		if from == "" {
			from = resolved.Source.Version
		}

		diff, err := vendoring.Diff(nil, &vendoring.DiffParams{
			Source: resolved.Source.Source,
			From:   from,
			To:     v.GetString("to"),
			File:   v.GetString("diff-file"),
		})
		if err != nil {
			return err
		}
		return data.Writeln(diff)
	},
}

func init() {
	vendorDiffParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Component to diff (required)"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform, helmfile, or packer)"),
		flags.WithStringFlag("from", "", "", "Starting ref (default: current pinned version)"),
		flags.WithStringFlag("to", "", "", "Ending ref (default: latest tag)"),
		flags.WithStringFlag("diff-file", "", "", "Restrict the diff to a single file path within the component"),
		flags.WithStringFlag("file", "", "", "Vendor manifest file (default: ./vendor.yaml)"),
	)
	vendorDiffParser.RegisterFlags(vendorDiffCmd)
	if err := vendorDiffParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorDiffCmd)
}
