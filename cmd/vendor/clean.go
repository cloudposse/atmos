package vendor

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	pkgtags "github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

var errModifiedVendorFiles = errors.New("modified vendor files were preserved; rerun with --force to delete them")

var vendorCleanParser *flags.StandardParser

// vendorCleanCmd removes files recorded in vendor.lock.yaml. It intentionally
// defaults to all artifacts because the lock gives it an exact ownership list.
var vendorCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove lock-owned vendored files",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := e.ProcessCommandLineArgs("terraform", cmd, args, nil)
		if err != nil {
			return err
		}
		component, err := cmd.Flags().GetString("component")
		if err != nil {
			return err
		}
		tagsCsv, err := cmd.Flags().GetString("tags")
		if err != nil {
			return err
		}
		filterTags := splitTags(tagsCsv)
		stack, err := cmd.Flags().GetString("stack")
		if err != nil {
			return err
		}
		labelsCsv, err := cmd.Flags().GetString("labels")
		if err != nil {
			return err
		}
		labels, err := pkgtags.ParseLabelsFlag(labelsCsv)
		if err != nil {
			return err
		}
		componentType, err := cmd.Flags().GetString("type")
		if err != nil {
			return err
		}
		file, err := cmd.Flags().GetString("file")
		if err != nil {
			return err
		}
		// --stack/--labels resolve components via ExecuteDescribeStacksScoped, which requires
		// atmosConfig.StackConfigFilesAbsolutePaths to be populated -- only they need
		// processStacks=true; --component/--tags operate on vendor.yaml/component.yaml manifests
		// directly and don't.
		config, err := cfg.InitCliConfig(info, stack != "" || len(labels) > 0)
		if err != nil {
			return err
		}
		if vendorSelectorGroupCount(component, stack, labels) > 1 {
			return errVendorSelectorsExclusive()
		}
		components, err := resolveVendorSelectorComponents(&VendorSelectorOptions{
			AtmosConfig:   &config,
			Component:     component,
			Tags:          filterTags,
			Stack:         stack,
			Labels:        labels,
			VendorFile:    file,
			ComponentType: componentType,
			TypeChanged:   cmd.Flags().Changed("type"),
		})
		if err != nil {
			return err
		}

		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		report, err := lockfile.CleanSelected(&config, components, force, dryRun)
		if err != nil {
			return err
		}
		for _, path := range report.Removed {
			if dryRun {
				ui.Infof("Would remove %s", path)
			} else {
				ui.Infof("Removed %s", path)
			}
		}
		if len(report.Conflicts) > 0 {
			for _, conflict := range report.Conflicts {
				ui.Warningf("Preserved modified vendor file %s", conflict.Path)
			}
			return fmt.Errorf("%w: %d", errModifiedVendorFiles, len(report.Conflicts))
		}
		return nil
	},
}

func init() {
	vendorCleanParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Clean only this component"),
		flags.WithStringFlag("type", "t", "terraform", componentTypeFlagHelp),
		flags.WithStringFlag("file", "", "", "Vendor manifest file (default: ./vendor.yaml)"),
		flags.WithStringFlag("tags", "", "", "Clean only components whose vendor.yaml source declares any of these tags (comma-separated, matches any)"),
		flags.WithStringFlag("stack", "s", "", "Clean only components belonging to the specified stack"),
		flags.WithStringFlag("labels", "", "", vendorLabelsFlagHelp),
		flags.WithBoolFlag("force", "", false, "Delete modified lock-owned files"),
		flags.WithBoolFlag("dry-run", "", false, "Show files that would be removed"),
	)
	vendorCleanParser.RegisterFlags(vendorCleanCmd)
	if err := vendorCleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorCleanCmd)
}
