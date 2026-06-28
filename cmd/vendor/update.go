package vendor

import (
	"strings"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// vendorUpdateCmd checks Git sources for newer allowed versions and updates the
// version fields in the vendor manifest(s), preserving formatting.
var vendorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update vendored component versions to the latest allowed release",
	Long: `Check each Git-backed source in the vendor manifest for a newer version (honoring
any per-source constraints) and update the version field in place, preserving
comments, anchors, and templates. Use --check for a dry run.`,
	Example: "atmos vendor update --check\natmos vendor update --component vpc\natmos vendor update --pull",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.updateRunE")()

		file, err := resolveVendorFile()
		if err != nil {
			return err
		}
		files, err := vendoring.CollectManifestFiles(file)
		if err != nil {
			return err
		}

		check, _ := cmd.Flags().GetBool("check")
		component, _ := cmd.Flags().GetString("component")
		tagsCSV, _ := cmd.Flags().GetString("tags")
		outdated, _ := cmd.Flags().GetBool("outdated")

		report, err := vendoring.Update(nil, &vendoring.UpdateParams{
			VendorFiles: files,
			Component:   component,
			Tags:        splitTags(tagsCSV),
			DryRun:      check,
		})
		if err != nil {
			return err
		}

		renderUpdateReport(report, check, outdated)

		if pull, _ := cmd.Flags().GetBool("pull"); pull && !check && report.UpdatedCount() > 0 {
			return runVendorPull(cmd, args, component)
		}
		return nil
	},
}

func init() {
	vendorUpdateCmd.Flags().StringP("component", "c", "", "Update only this component")
	vendorUpdateCmd.Flags().StringP("type", "t", "terraform", "Component type (terraform or helmfile)")
	vendorUpdateCmd.Flags().String("tags", "", "Update only components with any of these comma-separated tags")
	vendorUpdateCmd.Flags().Bool("check", false, "Dry run: show available updates without modifying files")
	vendorUpdateCmd.Flags().Bool("pull", false, "After updating versions, run 'atmos vendor pull'")
	vendorUpdateCmd.Flags().Bool("outdated", false, "Show only sources with an available update")
	vendorUpdateCmd.Flags().StringVar(&vendorFileFlag, "file", "", "Vendor manifest file (default: ./vendor.yaml)")
	// Flags consumed by 'vendor pull' when --pull is set.
	vendorUpdateCmd.Flags().StringP("stack", "s", "", "Only pull the specified stack (used with --pull)")
	vendorUpdateCmd.Flags().Bool("everything", false, "Pull all components (used with --pull)")
	vendorUpdateCmd.Flags().Bool("dry-run", false, "Simulate the pull (used with --pull)")

	vendorCmd.AddCommand(vendorUpdateCmd)
}

// renderUpdateReport prints the per-source results and a summary.
func renderUpdateReport(report *vendoring.UpdateReport, dryRun, outdated bool) {
	for _, r := range report.Results {
		switch r.Status {
		case vendoring.StatusUpdated:
			ui.Successf("%s (%s → %s)", r.Component, r.CurrentVersion, r.LatestVersion)
		case vendoring.StatusUpToDate:
			if !outdated {
				ui.Infof("%s (%s - up to date)", r.Component, r.CurrentVersion)
			}
		case vendoring.StatusSkipped:
			if !outdated {
				ui.Warningf("%s (skipped - %s)", r.Component, r.Reason)
			}
		}
	}

	n := report.UpdatedCount()
	switch {
	case n == 0:
		ui.Info("No updates available.")
	case dryRun:
		ui.Successf("Found %d update(s) available.", n)
	default:
		ui.Successf("Updated %d component(s).", n)
	}
}

// runVendorPull invokes the existing vendor pull execution after an update.
func runVendorPull(cmd *cobra.Command, args []string, component string) error {
	// When no component filter is set, pull everything.
	if component == "" {
		_ = cmd.Flags().Set("everything", "true")
	}
	return e.ExecuteVendorPullCmd(cmd, args)
}

// splitTags splits a comma-separated tag list, trimming whitespace and empties.
func splitTags(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(csv, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}
