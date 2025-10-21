package exec

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// Constants for commonly used strings.

// ExecuteVendorUpdateCmd executes `vendor update` commands.
func ExecuteVendorUpdateCmd(cmd *cobra.Command, args []string) error {
	// Initialize configuration
	atmosConfig, err := initVendorUpdateConfig(cmd, args)
	if err != nil {
		return err
	}

	// Parse and validate flags
	vendorFlags, err := parseVendorUpdateFlags(cmd)
	if err != nil {
		return err
	}

	// Execute the vendor update
	if err := executeVendorUpdate(&atmosConfig, vendorFlags); err != nil {
		return err
	}

	// Execute vendor pull if requested
	return executeVendorPullIfRequested(cmd, args, vendorFlags)
}

// initVendorUpdateConfig initializes the configuration for vendor update.
func initVendorUpdateConfig(cmd *cobra.Command, args []string) (schema.AtmosConfiguration, error) {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, err
	}

	// vendor update doesn't use stack flag
	processStacks := false

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return schema.AtmosConfiguration{}, fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	return atmosConfig, nil
}

// parseVendorUpdateFlags parses and validates vendor update flags.
func parseVendorUpdateFlags(cmd *cobra.Command) (*VendorFlags, error) {
	flags := cmd.Flags()

	checkOnly, err := flags.GetBool("check")
	if err != nil {
		return nil, err
	}

	component, err := flags.GetString("component")
	if err != nil {
		return nil, err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return nil, err
	}

	var tags []string
	if tagsCsv != "" {
		tags = strings.Split(tagsCsv, ",")
	}

	componentType, err := flags.GetString("type")
	if err != nil {
		return nil, err
	}

	vendorFlags := &VendorFlags{
		Component:     component,
		Tags:          tags,
		ComponentType: componentType,
		DryRun:        checkOnly,  // --check flag means dry-run
		Update:        !checkOnly, // Update unless --check is set
	}

	// Validate flags
	if err := validateVendorUpdateFlags(vendorFlags); err != nil {
		return nil, err
	}

	return vendorFlags, nil
}

// executeVendorPullIfRequested executes vendor pull if the --pull flag is set.
func executeVendorPullIfRequested(cmd *cobra.Command, args []string, vendorFlags *VendorFlags) error {
	flags := cmd.Flags()

	pullAfterUpdate, err := flags.GetBool("pull")
	if err != nil {
		return err
	}

	if !pullAfterUpdate || vendorFlags.DryRun {
		return nil
	}

	log.Info("Executing vendor pull for updated components...")

	// Get original flag values for passing to pull command
	tagsCsv, _ := flags.GetString("tags")

	// Create a new command for vendor pull with same filters
	pullCmd := &cobra.Command{}
	pullCmd.Flags().StringP("component", "c", vendorFlags.Component, "")
	pullCmd.Flags().String("tags", tagsCsv, "")
	pullCmd.Flags().StringP("type", "t", vendorFlags.ComponentType, "")

	if err := ExecuteVendorPullCommand(pullCmd, args); err != nil {
		return fmt.Errorf("version references updated but pull failed: %w", err)
	}

	log.Info("Successfully updated version references and pulled new component versions")
	return nil
}

// validateVendorUpdateFlags validates the vendor update command flags.
func validateVendorUpdateFlags(flg *VendorFlags) error {
	// Component and tags can be used together for vendor update
	// No additional validation needed beyond basic checks
	return nil
}

// executeVendorUpdate performs the actual vendor update logic.
func executeVendorUpdate(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	// Determine the vendor config file path
	vendorConfigFileName := cfg.AtmosVendorConfigFileName
	if atmosConfig.Vendor.BasePath != "" {
		vendorConfigFileName = atmosConfig.Vendor.BasePath
	}

	// Read the main vendor config
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(
		atmosConfig,
		vendorConfigFileName,
		true,
	)
	if err != nil {
		return err
	}

	if !vendorConfigExists {
		// Try component vendor config if no main vendor config
		if flg.Component != "" {
			return executeComponentVendorUpdate(atmosConfig, flg)
		}
		return fmt.Errorf("%w: %s", errUtils.ErrVendorConfigFileNotFound, vendorConfigFileName)
	}

	// Process all imports and get sources with file mappings
	sources, importedFiles, err := processVendorImportsWithFileTracking(
		atmosConfig,
		foundVendorConfigFile,
		vendorConfig.Spec.Imports,
		vendorConfig.Spec.Sources,
		[]string{foundVendorConfigFile},
	)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		fmt.Println("No vendor sources configured")
		return nil
	}

	// Filter sources based on component and tags
	filteredSources := filterSources(sources, flg.Component, flg.Tags)

	if len(filteredSources) == 0 {
		reportNoSourcesFound(flg)
		return nil
	}

	// Check for updates and display results
	return checkAndUpdateVendorVersions(filteredSources, importedFiles, flg.DryRun, foundVendorConfigFile)
}

// executeComponentVendorUpdate handles vendor update for component.yaml files.
func executeComponentVendorUpdate(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	componentType := flg.ComponentType
	if componentType == "" {
		componentType = "terraform"
	}

	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		flg.Component,
		componentType,
	)
	if err != nil {
		return err
	}

	// Create a source from the component config
	source := schema.AtmosVendorSource{
		Component: flg.Component,
		Source:    config.Spec.Source.Uri,
		Version:   config.Spec.Source.Version,
	}

	// Check for updates
	sources := []schema.AtmosVendorSource{source}
	fileMap := map[string]string{flg.Component: path + "/component.yaml"}

	return checkAndUpdateVendorVersions(sources, fileMap, flg.DryRun, path+"/component.yaml")
}

// processVendorImportsWithFileTracking processes imports and tracks which file each source comes from.
func processVendorImportsWithFileTracking(
	atmosConfig *schema.AtmosConfiguration,
	vendorConfigFile string,
	imports []string,
	sources []schema.AtmosVendorSource,
	allImports []string,
) ([]schema.AtmosVendorSource, map[string]string, error) {
	// This will map component names to their source files
	fileMap := make(map[string]string)

	// Process imports recursively
	allSources, _, err := processVendorImports(
		atmosConfig,
		vendorConfigFile,
		imports,
		sources,
		allImports,
	)
	if err != nil {
		return nil, nil, err
	}

	// Build the file map from the sources
	for i := range allSources {
		source := &allSources[i]
		componentName := source.Component
		if componentName == "" {
			componentName = extractComponentNameFromSource(source.Source)
		}

		// Use the File field that was set during import processing
		if source.File != "" {
			fileMap[componentName] = source.File
		} else {
			fileMap[componentName] = vendorConfigFile
		}
	}

	return allSources, fileMap, nil
}

// vendorUpdateHelper contains helper methods for vendor update operations.
type vendorUpdateHelper struct{}

// newVendorUpdateHelper creates a new vendor update helper.
func newVendorUpdateHelper() *vendorUpdateHelper {
	return &vendorUpdateHelper{}
}

// prepareDiffPackages converts vendor sources to diff packages for the TUI.
func (h *vendorUpdateHelper) prepareDiffPackages(sources []schema.AtmosVendorSource) []pkgVendorDiff {
	diffPackages := make([]pkgVendorDiff, 0, len(sources))

	for i := range sources {
		componentName := h.extractComponentName(&sources[i])
		currentVersion := h.extractCurrentVersion(&sources[i])

		// Skip templated versions
		if h.isTemplatedVersion(currentVersion) {
			// Skip silently - logging happens elsewhere
			continue
		}

		diffPackages = append(diffPackages, pkgVendorDiff{
			name:           componentName,
			currentVersion: currentVersion,
			source:         sources[i],
			outdatedOnly:   false,
		})
	}

	return diffPackages
}

// extractComponentName extracts the component name from a vendor source.
func (h *vendorUpdateHelper) extractComponentName(source *schema.AtmosVendorSource) string {
	if source.Component != "" {
		return source.Component
	}
	return extractComponentNameFromSource(source.Source)
}

// extractCurrentVersion extracts the current version from a vendor source.
func (h *vendorUpdateHelper) extractCurrentVersion(source *schema.AtmosVendorSource) string {
	if source.Version != "" {
		return source.Version
	}
	return defaultVersionLatest
}

// isTemplatedVersion checks if a version contains template markers.
func (h *vendorUpdateHelper) isTemplatedVersion(version string) bool {
	return strings.Contains(version, templateStartMarker)
}

// groupUpdatesByFile organizes version updates by configuration file.
func (h *vendorUpdateHelper) groupUpdatesByFile(
	sources []schema.AtmosVendorSource,
	fileMap map[string]string,
	mainConfigFile string,
) map[string]map[string]string {
	updatesByFile := make(map[string]map[string]string)

	fmt.Println("\nChecking for version updates...")

	for i := range sources {
		componentName := h.extractComponentName(&sources[i])

		// Skip templated versions
		if h.isTemplatedVersion(sources[i].Version) {
			continue
		}

		// Check for updates
		updateAvailable, latestVersion, err := checkForVendorUpdates(&sources[i], true)
		if err != nil {
			// Skip silently - error will be logged elsewhere
			continue
		}

		if updateAvailable && latestVersion != "" {
			configFile := h.determineConfigFile(componentName, fileMap, mainConfigFile)
			h.addUpdateToFile(updatesByFile, configFile, componentName, latestVersion)
		}
	}

	return updatesByFile
}

// determineConfigFile determines which configuration file to update.
func (h *vendorUpdateHelper) determineConfigFile(
	componentName string,
	fileMap map[string]string,
	mainConfigFile string,
) string {
	if configFile, exists := fileMap[componentName]; exists {
		return configFile
	}
	return mainConfigFile
}

// addUpdateToFile adds an update to the appropriate file's update map.
func (h *vendorUpdateHelper) addUpdateToFile(
	updatesByFile map[string]map[string]string,
	configFile string,
	componentName string,
	latestVersion string,
) {
	if updatesByFile[configFile] == nil {
		updatesByFile[configFile] = make(map[string]string)
	}
	updatesByFile[configFile][componentName] = latestVersion
}

// applyUpdatesToFiles applies version updates to configuration files.
func (h *vendorUpdateHelper) applyUpdatesToFiles(updatesByFile map[string]map[string]string) error {
	totalUpdates := 0

	for configFile, updates := range updatesByFile {
		if len(updates) == 0 {
			continue
		}

		fmt.Printf("\nUpdating %d components in %s...\n", len(updates), configFile)

		if err := updateVendorConfigFile(updates, configFile); err != nil {
			return fmt.Errorf("error updating %s: %w", configFile, err)
		}

		totalUpdates += len(updates)
	}

	h.printUpdateSummary(totalUpdates, len(updatesByFile))
	return nil
}

// printUpdateSummary prints a summary of the updates performed.
func (h *vendorUpdateHelper) printUpdateSummary(totalUpdates int, fileCount int) {
	if totalUpdates > 0 {
		fmt.Printf("\nSuccessfully updated %d components across %d files\n", totalUpdates, fileCount)
	} else {
		fmt.Println("\nAll vendor dependencies are up to date!")
	}
}

// printOperationHeader prints the appropriate header for the operation.
func (h *vendorUpdateHelper) printOperationHeader(dryRun bool) {
	if dryRun {
		fmt.Println("Checking for vendor updates...")
	} else {
		fmt.Println("Updating vendor configurations...")
	}
	fmt.Println()
}

// checkAndUpdateVendorVersions checks for version updates and optionally updates the files.
func checkAndUpdateVendorVersions(
	sources []schema.AtmosVendorSource,
	fileMap map[string]string,
	dryRun bool,
	mainConfigFile string,
) error {
	helper := newVendorUpdateHelper()

	// Print operation header
	helper.printOperationHeader(dryRun)

	// Prepare diff packages for version checking
	diffPackages := helper.prepareDiffPackages(sources)

	// Execute version check using the TUI model
	if err := executeVendorModel(diffPackages, true, &schema.AtmosConfiguration{}); err != nil {
		return fmt.Errorf("failed to check vendor updates: %w", err)
	}

	// If dry-run, we're done
	if dryRun {
		return nil
	}

	// Group updates by configuration file
	updatesByFile := helper.groupUpdatesByFile(sources, fileMap, mainConfigFile)

	// Apply updates to files
	return helper.applyUpdatesToFiles(updatesByFile)
}

// reportNoSourcesFound reports when no vendor sources are found based on filters.
func reportNoSourcesFound(flg *VendorFlags) {
	switch {
	case flg.Component != "":
		fmt.Printf("No vendor sources found for component: %s\n", flg.Component)
	case len(flg.Tags) > 0:
		fmt.Printf("No vendor sources found for tags: %v\n", flg.Tags)
	default:
		fmt.Println("No vendor sources found")
	}
}
