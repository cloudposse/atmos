package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// vendorDiffHelper provides methods for comparing and displaying vendor differences.
type vendorDiffHelper struct{}

// newVendorDiffHelper creates a new vendor diff helper.
func newVendorDiffHelper() *vendorDiffHelper {
	return &vendorDiffHelper{}
}

// compareAndDisplayVendorDiffs compares local and remote versions and displays differences.
func compareAndDisplayVendorDiffs(sources []schema.AtmosVendorSource, updateVendorFile bool, outdatedOnly bool, vendorConfigFile string) error {
	helper := newVendorDiffHelper()

	// Print header
	helper.printHeader()

	// Prepare diff packages for comparison
	diffPackages := helper.prepareDiffPackages(sources, outdatedOnly)

	// Execute vendor model for comparison
	if err := executeVendorModel(diffPackages, true, &schema.AtmosConfiguration{}); err != nil {
		return fmt.Errorf("vendor diff failed: %w", err)
	}

	// Handle update request if specified
	if updateVendorFile {
		return helper.handleUpdateRequest(sources, vendorConfigFile)
	}

	return nil
}

// printHeader prints the operation header.
func (h *vendorDiffHelper) printHeader() {
	fmt.Println("Checking for vendor updates...")
	fmt.Println()
}

// prepareDiffPackages converts sources to diff packages for the TUI.
func (h *vendorDiffHelper) prepareDiffPackages(sources []schema.AtmosVendorSource, outdatedOnly bool) []pkgVendorDiff {
	diffPackages := make([]pkgVendorDiff, 0, len(sources))

	for i := range sources {
		source := &sources[i]
		pkg := h.createDiffPackage(source, outdatedOnly)
		diffPackages = append(diffPackages, pkg)
	}

	return diffPackages
}

// createDiffPackage creates a diff package from a vendor source.
func (h *vendorDiffHelper) createDiffPackage(source *schema.AtmosVendorSource, outdatedOnly bool) pkgVendorDiff {
	componentName := h.extractComponentName(source)
	currentVersion := h.extractCurrentVersion(source)

	return pkgVendorDiff{
		name:           componentName,
		currentVersion: currentVersion,
		source:         *source,
		outdatedOnly:   outdatedOnly,
	}
}

// extractComponentName extracts the component name from a source.
func (h *vendorDiffHelper) extractComponentName(source *schema.AtmosVendorSource) string {
	if source.Component != "" {
		return source.Component
	}
	return extractComponentNameFromSource(source.Source)
}

// extractCurrentVersion extracts the current version from a source.
func (h *vendorDiffHelper) extractCurrentVersion(source *schema.AtmosVendorSource) string {
	if source.Version != "" {
		return source.Version
	}
	return "latest"
}

// handleUpdateRequest handles the vendor file update request.
func (h *vendorDiffHelper) handleUpdateRequest(sources []schema.AtmosVendorSource, vendorConfigFile string) error {
	// Collect updates
	updatedVersions := h.collectUpdates(sources)

	if len(updatedVersions) == 0 {
		fmt.Println("\nNo updates available.")
		return nil
	}

	// Apply updates to vendor config file
	return h.applyUpdates(updatedVersions, vendorConfigFile)
}

// collectUpdates collects available version updates.
func (h *vendorDiffHelper) collectUpdates(sources []schema.AtmosVendorSource) map[string]string {
	updatedVersions := make(map[string]string)

	fmt.Println("\nCollecting update information...")

	for i := range sources {
		source := &sources[i]
		componentName := h.extractComponentName(source)

		// Check for updates using the existing logic
		updateAvailable, latestInfo, err := checkForVendorUpdates(&sources[i], true)
		if err != nil {
			continue // Skip components with errors
		}

		if updateAvailable && latestInfo != "" {
			updatedVersions[componentName] = latestInfo
		}
	}

	return updatedVersions
}

// applyUpdates applies the collected updates to the vendor config file.
func (h *vendorDiffHelper) applyUpdates(updatedVersions map[string]string, vendorConfigFile string) error {
	fmt.Printf("\nUpdating %d components in vendor config...\n", len(updatedVersions))

	if err := updateVendorConfigFile(updatedVersions, vendorConfigFile); err != nil {
		return fmt.Errorf("failed to update vendor config file %s: %w", vendorConfigFile, err)
	}

	// Pull the updated components
	fmt.Println("\nPulling updated components...")
	if err := ExecuteVendorPullCmd(nil, []string{}); err != nil {
		return fmt.Errorf("version references updated but pull failed: %w", err)
	}

	fmt.Println("\nSuccessfully updated vendor config and pulled new versions.")
	return nil
}
