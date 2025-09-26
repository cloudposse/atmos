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
const (
	componentKey = "component"
	latestTag    = "latest"
	gitType      = "git"
)

// ExecuteVendorUpdateCmd executes `vendor update` commands.
func ExecuteVendorUpdateCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	// vendor update doesn't use stack flag
	processStacks := false

	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	// Parse vendor update specific flags
	checkOnly, err := flags.GetBool("check")
	if err != nil {
		return err
	}

	pullAfterUpdate, err := flags.GetBool("pull")
	if err != nil {
		return err
	}

	component, err := flags.GetString("component")
	if err != nil {
		return err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return err
	}

	var tags []string
	if tagsCsv != "" {
		tags = strings.Split(tagsCsv, ",")
	}

	componentType, err := flags.GetString("type")
	if err != nil {
		return err
	}

	// Create vendor flags structure
	vendorFlags := VendorFlags{
		Component:     component,
		Tags:          tags,
		ComponentType: componentType,
		DryRun:        checkOnly,  // --check flag means dry-run
		Update:        !checkOnly, // Update unless --check is set
	}

	// Validate flags
	if err := validateVendorUpdateFlags(&vendorFlags); err != nil {
		return err
	}

	// Execute the vendor update
	err = executeVendorUpdate(&atmosConfig, &vendorFlags)
	if err != nil {
		return err
	}

	// If --pull flag is set and we're not in check-only mode, execute vendor pull
	if pullAfterUpdate && !checkOnly {
		log.Info("Executing vendor pull for updated components...")

		// Create a new command for vendor pull with same filters
		pullCmd := &cobra.Command{}
		pullCmd.Flags().StringP("component", "c", component, "")
		pullCmd.Flags().String("tags", tagsCsv, "")
		pullCmd.Flags().StringP("type", "t", componentType, "")

		err = ExecuteVendorPullCommand(pullCmd, args)
		if err != nil {
			return fmt.Errorf("version references updated but pull failed: %w", err)
		}

		log.Info("Successfully updated version references and pulled new component versions")
	}

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
		if flg.Component != "" {
			fmt.Printf("No vendor sources found for component: %s\n", flg.Component)
		} else if len(flg.Tags) > 0 {
			fmt.Printf("No vendor sources found for tags: %v\n", flg.Tags)
		} else {
			fmt.Println("No vendor sources found")
		}
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
	for _, source := range allSources {
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

// checkAndUpdateVendorVersions checks for version updates and optionally updates the files.
func checkAndUpdateVendorVersions(
	sources []schema.AtmosVendorSource,
	fileMap map[string]string,
	dryRun bool,
	mainConfigFile string,
) error {
	// Print header
	if dryRun {
		fmt.Println("Checking for vendor updates...")
	} else {
		fmt.Println("Updating vendor configurations...")
	}
	fmt.Println()

	// Convert sources to diff packages for the TUI
	diffPackages := make([]pkgVendorDiff, 0, len(sources))
	for _, source := range sources {
		componentName := source.Component
		if componentName == "" {
			componentName = extractComponentNameFromSource(source.Source)
		}

		currentVersion := source.Version
		if currentVersion == "" {
			currentVersion = "latest"
		}

		// Skip templated versions
		if strings.Contains(currentVersion, "{{") {
			log.Warn("Skipping templated version", "component", componentName, "version", currentVersion)
			continue
		}

		diffPackages = append(diffPackages, pkgVendorDiff{
			name:           componentName,
			currentVersion: currentVersion,
			source:         source,
			outdatedOnly:   false,
		})
	}

	// Use the vendor model TUI to check versions
	err := executeVendorModel(diffPackages, true, &schema.AtmosConfiguration{})
	if err != nil {
		return fmt.Errorf("failed to check vendor updates: %w", err)
	}

	// If not dry-run, update the configuration files
	if !dryRun {
		// Group updates by file
		updatesByFile := make(map[string]map[string]string)

		fmt.Println("\nChecking for version updates...")

		for _, source := range sources {
			componentName := source.Component
			if componentName == "" {
				componentName = extractComponentNameFromSource(source.Source)
			}

			// Skip templated versions
			if strings.Contains(source.Version, "{{") {
				continue
			}

			// Check for updates
			updateAvailable, latestVersion, err := checkForVendorUpdates(source, true)
			if err != nil {
				log.Debug("Error checking updates", componentKey, componentName, "error", err)
				continue
			}

			if updateAvailable && latestVersion != "" {
				// Determine which file to update
				configFile := fileMap[componentName]
				if configFile == "" {
					configFile = mainConfigFile
				}

				if updatesByFile[configFile] == nil {
					updatesByFile[configFile] = make(map[string]string)
				}
				updatesByFile[configFile][componentName] = latestVersion
			}
		}

		// Update each file with its respective updates
		totalUpdates := 0
		for configFile, updates := range updatesByFile {
			if len(updates) > 0 {
				fmt.Printf("\nUpdating %d components in %s...\n", len(updates), configFile)

				if err := updateVendorConfigFile(updates, configFile); err != nil {
					return fmt.Errorf("error updating %s: %w", configFile, err)
				}

				totalUpdates += len(updates)
			}
		}

		if totalUpdates > 0 {
			fmt.Printf("\nSuccessfully updated %d components across %d files\n", totalUpdates, len(updatesByFile))
		} else {
			fmt.Println("\nAll vendor dependencies are up to date!")
		}
	}

	return nil
}
