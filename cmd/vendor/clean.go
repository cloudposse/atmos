package vendor

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
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
		config, err := cfg.InitCliConfig(info, false)
		if err != nil {
			return err
		}
		component, err := cmd.Flags().GetString("component")
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
		report, err := lockfile.Clean(&config, component, force, dryRun)
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
		flags.WithBoolFlag("force", "", false, "Delete modified lock-owned files"),
		flags.WithBoolFlag("dry-run", "", false, "Show files that would be removed"),
	)
	vendorCleanParser.RegisterFlags(vendorCleanCmd)
	if err := vendorCleanParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorCleanCmd)
}
