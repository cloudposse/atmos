package vendor

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgtags "github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

var vendorDiffParser *flags.StandardParser

// vendorDiffCmd shows the Git diff between two versions of a vendored component.
var vendorDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the Git diff between two versions of a vendored component",
	Long: `Show the changes between two versions (tags, branches, or commits) of one or more vendored
Git components, without a local checkout. Defaults --from to each component's current pinned
version and --to to the latest tag. Selects exactly one component via --component, or several via
--tags (vendor.yaml's own declared source tags), --stack, or --labels (stack-resolved component
metadata.labels) -- see 'atmos vendor pull --help' for the distinction between --tags and
--stack/--labels.`,
	Example: "atmos vendor diff --component vpc\natmos vendor diff -c vpc --from 1.0.0 --to 2.0.0\natmos vendor diff --tags networking\natmos vendor diff --stack dev-us-west-2 --labels tier=1",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.diffRunE")()

		v := viper.GetViper()
		if err := vendorDiffParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		components, err := resolveDiffComponents(cmd, v)
		if err != nil {
			return err
		}

		if len(components) == 1 {
			diff, diffErr := diffOneComponent(v, components[0])
			if diffErr != nil {
				return diffErr
			}
			return data.Writeln(diff)
		}

		return diffManyComponents(v, components)
	},
}

// resolveDiffComponents resolves exactly one selector (--component, --tags, --stack/--labels) into
// the list of component names to diff.
func resolveDiffComponents(cmd *cobra.Command, v *viper.Viper) ([]string, error) {
	component := v.GetString("component")
	filterTags := splitTags(v.GetString("tags"))
	stack := v.GetString("stack")
	labels, err := pkgtags.ParseLabelsFlag(v.GetString("labels"))
	if err != nil {
		return nil, err
	}

	switch vendorSelectorGroupCount(component, filterTags, stack, labels) {
	case 0:
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation("One of --component, --tags, --stack, or --labels is required.").
			WithHint("Specify which component(s) to diff, e.g. atmos vendor diff --component vpc.").
			Err()
	case 1:
		// Exactly one selector -- proceed.
	default:
		return nil, errVendorSelectorsExclusive()
	}

	atmosConfig, cfgErr := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if cfgErr != nil {
		return nil, cfgErr
	}

	// resolveVendorSelectorComponents itself errors when a real selector (--tags/--stack/--labels)
	// matches nothing; --component always resolves to itself and can't hit that path.
	return resolveVendorSelectorComponents(&VendorSelectorOptions{
		AtmosConfig:   &atmosConfig,
		Component:     component,
		Tags:          filterTags,
		Stack:         stack,
		Labels:        labels,
		VendorFile:    v.GetString("file"),
		ComponentType: v.GetString("type"),
		TypeChanged:   cmd.Flags().Changed("type"),
	})
}

// diffOneComponent resolves and diffs a single component, preserving the original (pre-batch)
// output shape exactly -- no header, just the raw diff.
func diffOneComponent(v *viper.Viper, component string) (string, error) {
	resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{
		VendorFile:    v.GetString("file"),
		Component:     component,
		ComponentType: v.GetString("type"),
	})
	if err != nil {
		return "", err
	}

	from := v.GetString("from")
	if from == "" {
		from = resolved.Source.Version
	}

	return vendoring.Diff(nil, &vendoring.DiffParams{
		Source: resolved.Source.Source,
		From:   from,
		To:     v.GetString("to"),
		File:   v.GetString("diff-file"),
	})
}

// diffManyComponents diffs each of components in turn, writing each under a "## <component>"
// header. A per-component resolution/diff failure doesn't abort the remaining components -- errors
// are joined and returned after every component has had a chance to run, matching the multi
// -component error-handling convention used elsewhere in this package (e.g. handleStackVendor,
// handleVendorPullSweep).
func diffManyComponents(v *viper.Viper, components []string) error {
	var errs []error
	for _, component := range components {
		diff, err := diffOneComponent(v, component)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", component, err))
			continue
		}
		if err := data.Writeln(fmt.Sprintf("## %s\n", component)); err != nil {
			return err
		}
		if err := data.Writeln(diff); err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

func init() {
	vendorDiffParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Component to diff"),
		flags.WithStringFlag("type", "t", "terraform", componentTypeFlagHelp),
		flags.WithStringFlag("from", "", "", "Starting ref (default: current pinned version)"),
		flags.WithStringFlag("to", "", "", "Ending ref (default: latest tag)"),
		flags.WithStringFlag("diff-file", "", "", "Restrict the diff to a single file path within the component"),
		flags.WithStringFlag("file", "", "", "Vendor manifest file (default: ./vendor.yaml)"),
		flags.WithStringFlag("tags", "", "", "Diff every component whose vendor.yaml source declares any of these tags (comma-separated, matches any)"),
		flags.WithStringFlag("stack", "s", "", "Diff every component belonging to the specified stack"),
		flags.WithStringFlag("labels", "", "", vendorLabelsFlagHelp),
	)
	vendorDiffParser.RegisterFlags(vendorDiffCmd)
	if err := vendorDiffParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorDiffCmd)
}
