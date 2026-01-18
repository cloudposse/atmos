package vendoring

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// updateFlags holds flags specific to vendor update command.
type updateFlags struct {
	Check         bool
	Pull          bool
	Component     string
	Tags          []string
	ComponentType string
	Outdated      bool
}

// updateResult holds the result of checking a source for updates.
type updateResult struct {
	Component      string
	Source         string
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	VendorSource   schema.AtmosVendorSource
	FilePath       string // The vendor config file where this source is defined.
}

// Update executes the vendor update operation with typed params.
func Update(atmosConfig *schema.AtmosConfiguration, params *UpdateParams) error {
	defer perf.Track(atmosConfig, "vendor.Update")()

	// Convert params to internal updateFlags format.
	var tags []string
	if params.Tags != "" {
		tags = splitAndTrim(params.Tags, ",")
	}

	flags := &updateFlags{
		Check:         params.Check,
		Pull:          params.Pull,
		Component:     params.Component,
		Tags:          tags,
		ComponentType: params.ComponentType,
		Outdated:      params.Outdated,
	}

	// Execute vendor update.
	return executeVendorUpdate(atmosConfig, flags)
}

// executeVendorUpdate performs the vendor update logic.
func executeVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *updateFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeVendorUpdate")()

	// Load and filter sources.
	sources, foundVendorConfigFile, err := loadAndFilterSources(atmosConfig, flags)
	if err != nil {
		return err
	}
	if sources == nil {
		return nil // No sources to process (warning already printed).
	}

	// Check each source for updates.
	updates := checkAllSourcesForUpdates(atmosConfig, sources, foundVendorConfigFile, flags.Outdated)

	// Display results.
	displayUpdateResults(updates, flags.Check)

	// If --check flag is set, stop here (dry-run mode).
	if flags.Check {
		return nil
	}

	// Apply updates and optionally pull.
	return applyUpdatesAndPull(atmosConfig, updates, foundVendorConfigFile, flags)
}

// loadAndFilterSources loads vendor config and filters sources based on flags.
func loadAndFilterSources(
	atmosConfig *schema.AtmosConfiguration,
	flags *updateFlags,
) ([]schema.AtmosVendorSource, string, error) {
	defer perf.Track(atmosConfig, "vendor.loadAndFilterSources")()

	// Determine the vendor config file path.
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config.
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return nil, "", err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config.
		if flags.Component != "" {
			return nil, "", executeComponentVendorUpdate(atmosConfig, flags)
		}
		return nil, "", fmt.Errorf("%w: %s", errUtils.ErrVendorConfigNotFound, vendorConfigFileName)
	}

	// Get all sources from vendor config.
	sources := vendorConfig.Spec.Sources

	// Filter sources by component name if --component flag provided.
	if flags.Component != "" {
		sources = filterSourcesByComponent(sources, flags.Component)
		if len(sources) == 0 {
			return nil, "", fmt.Errorf("%w: %s", errUtils.ErrVendorComponentNotFound, flags.Component)
		}
	}

	// Filter sources by tags if --tags flag provided.
	if len(flags.Tags) > 0 {
		sources = filterSourcesByTags(sources, flags.Tags)
		if len(sources) == 0 {
			_ = ui.Warning("No sources match the specified tags")
			return nil, foundVendorConfigFile, nil
		}
	}

	return sources, foundVendorConfigFile, nil
}

// checkAllSourcesForUpdates checks all sources for available updates.
func checkAllSourcesForUpdates(
	atmosConfig *schema.AtmosConfiguration,
	sources []schema.AtmosVendorSource,
	vendorConfigFile string,
	outdatedOnly bool,
) []updateResult {
	defer perf.Track(atmosConfig, "vendor.checkAllSourcesForUpdates")()

	var updates []updateResult
	for i := range sources {
		result, err := checkSourceForUpdates(atmosConfig, &sources[i], vendorConfigFile)
		if err != nil {
			// Log warning and continue with other sources.
			_ = ui.Warning(fmt.Sprintf("Failed to check %s for updates: %v", sources[i].Component, err))
			continue
		}

		// If outdatedOnly is set, only include sources that have updates.
		if outdatedOnly && !result.HasUpdate {
			continue
		}

		updates = append(updates, result)
	}

	return updates
}

// applyUpdatesAndPull applies version updates to YAML and optionally pulls.
func applyUpdatesAndPull(
	atmosConfig *schema.AtmosConfiguration,
	updates []updateResult,
	vendorConfigFile string,
	flags *updateFlags,
) error {
	defer perf.Track(atmosConfig, "vendor.applyUpdatesAndPull")()

	updatedCount := 0
	for i := range updates {
		if updates[i].HasUpdate {
			err := updateYAMLVersion(atmosConfig, updates[i].FilePath, updates[i].Component, updates[i].LatestVersion)
			if err != nil {
				return fmt.Errorf("%w: failed to update %s: %w", errUtils.ErrYAMLUpdateFailed, updates[i].Component, err)
			}
			updatedCount++
		}
	}

	if updatedCount > 0 {
		_ = ui.Success(fmt.Sprintf("Updated %d component(s) in %s", updatedCount, vendorConfigFile))
	}

	// If --pull flag is set, execute vendor pull.
	if flags.Pull && updatedCount > 0 {
		_ = ui.Info("Pulling updated components...")
		pullParams := &PullParams{
			Component:     flags.Component,
			Tags:          strings.Join(flags.Tags, ","),
			ComponentType: flags.ComponentType,
		}
		if err := Pull(atmosConfig, pullParams); err != nil {
			return fmt.Errorf("%w: vendor pull failed: %w", errUtils.ErrVendorPullFailed, err)
		}
	}

	return nil
}

// executeComponentVendorUpdate handles vendor update for component.yaml files.
// NOTE: Component-level vendor update is planned for a future release.
func executeComponentVendorUpdate(atmosConfig *schema.AtmosConfiguration, flags *updateFlags) error {
	defer perf.Track(atmosConfig, "vendor.executeComponentVendorUpdate")()

	// TODO: Implement component vendor update:
	// 1. Read component.yaml from components/{flags.ComponentType}/{flags.Component}/component.yaml.
	// 2. Extract version and source information.
	// 3. Check for updates using Git provider.
	// 4. Update component.yaml if newer version available.
	_ = flags

	return errUtils.ErrNotImplemented
}

// splitAndTrim splits a string by delimiter and trims whitespace from each element.
func splitAndTrim(s, delimiter string) []string {
	if s == "" {
		return nil
	}

	parts := []string{}
	for _, part := range strings.Split(s, delimiter) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	return parts
}

// filterSourcesByComponent filters sources to match a specific component name.
func filterSourcesByComponent(sources []schema.AtmosVendorSource, component string) []schema.AtmosVendorSource {
	defer perf.Track(nil, "vendor.filterSourcesByComponent")()

	var filtered []schema.AtmosVendorSource
	for i := range sources {
		if sources[i].Component == component {
			filtered = append(filtered, sources[i])
		}
	}
	return filtered
}

// filterSourcesByTags filters sources that have at least one of the specified tags.
func filterSourcesByTags(sources []schema.AtmosVendorSource, tags []string) []schema.AtmosVendorSource {
	defer perf.Track(nil, "vendor.filterSourcesByTags")()

	if len(tags) == 0 {
		return sources
	}

	// Build a set of target tags for O(1) lookup.
	tagSet := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tagSet[tag] = struct{}{}
	}

	var filtered []schema.AtmosVendorSource
	for i := range sources {
		for _, sourceTag := range sources[i].Tags {
			if _, found := tagSet[sourceTag]; found {
				filtered = append(filtered, sources[i])
				break
			}
		}
	}
	return filtered
}

// checkSourceForUpdates checks if a source has a newer version available.
func checkSourceForUpdates(
	atmosConfig *schema.AtmosConfiguration,
	source *schema.AtmosVendorSource,
	filePath string,
) (updateResult, error) {
	defer perf.Track(atmosConfig, "vendor.checkSourceForUpdates")()

	result := updateResult{
		Component:    source.Component,
		Source:       source.Source,
		VendorSource: *source,
		FilePath:     filePath,
	}

	// Get current version from source.
	currentVersion := source.Version
	if currentVersion == "" {
		// No version specified - cannot check for updates.
		result.CurrentVersion = "(no version)"
		return result, nil
	}
	result.CurrentVersion = currentVersion

	// Extract Git URI from source.
	gitURI := version.ExtractGitURI(source.Source)
	if gitURI == "" {
		return result, fmt.Errorf("%w: empty Git URI for %s", errUtils.ErrInvalidVendorSource, source.Component)
	}

	// Get available tags from remote.
	tags, err := version.GetGitRemoteTags(gitURI)
	if err != nil {
		return result, fmt.Errorf("%w: %w", errUtils.ErrGitLsRemoteFailed, err)
	}

	if len(tags) == 0 {
		// No tags available.
		result.LatestVersion = currentVersion
		return result, nil
	}

	// Apply version constraints if defined, otherwise find latest semver tag.
	var latestVersion string
	if source.Constraints != nil {
		latestVersion, err = version.ResolveVersionConstraints(tags, source.Constraints)
		if err != nil {
			return result, fmt.Errorf("%w: %w", errUtils.ErrVersionConstraintsFailed, err)
		}
	} else {
		_, latestVersion = version.FindLatestSemVerTag(tags)
		if latestVersion == "" {
			// No valid semver tags found - use first tag as fallback.
			latestVersion = tags[0]
		}
	}

	result.LatestVersion = latestVersion
	result.HasUpdate = latestVersion != currentVersion

	return result, nil
}

// displayUpdateResults displays the update check results to the user.
func displayUpdateResults(updates []updateResult, isDryRun bool) {
	defer perf.Track(nil, "vendor.displayUpdateResults")()

	if len(updates) == 0 {
		_ = ui.Info("No components to check for updates")
		return
	}

	if isDryRun {
		_ = ui.Info("Checking for updates (dry-run mode)...")
	}

	hasUpdates := false
	upToDate := 0

	for i := range updates {
		if updates[i].HasUpdate {
			hasUpdates = true
			_ = ui.Success(fmt.Sprintf("%s: %s → %s", updates[i].Component, updates[i].CurrentVersion, updates[i].LatestVersion))
		} else {
			upToDate++
		}
	}

	// Show summary of up-to-date components.
	if upToDate > 0 {
		if upToDate == 1 {
			_ = ui.Info("1 component is up to date")
		} else {
			_ = ui.Info(fmt.Sprintf("%d components are up to date", upToDate))
		}
	}

	if !hasUpdates {
		_ = ui.Info("All components are up to date")
	} else if isDryRun {
		_ = ui.Info("Run without --check to apply updates")
	}
}
