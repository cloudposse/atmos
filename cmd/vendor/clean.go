package vendor

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

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
			return fmt.Errorf("%d modified vendor files were preserved; rerun with --force to delete them", len(report.Conflicts))
		}
		return nil
	},
}

func init() {
	vendorCleanCmd.Flags().StringP("component", "c", "", "Clean only this component")
	vendorCleanCmd.Flags().Bool("force", false, "Delete modified lock-owned files")
	vendorCleanCmd.Flags().Bool("dry-run", false, "Show files that would be removed")
	vendorCmd.AddCommand(vendorCleanCmd)
}
